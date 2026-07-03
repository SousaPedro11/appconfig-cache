package appconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type mockDynamoDB struct {
	getItemFunc func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error)
	putItemFunc func(ctx context.Context, params *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error)
	queryFunc   func(ctx context.Context, params *dynamodb.QueryInput) (*dynamodb.QueryOutput, error)
}

func (m *mockDynamoDB) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return m.getItemFunc(ctx, params)
}

func (m *mockDynamoDB) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return m.putItemFunc(ctx, params)
}

func (m *mockDynamoDB) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return m.queryFunc(ctx, params)
}

func TestSharedCircuitBreaker_GetState(t *testing.T) {
	expectedOpen := circuitState{
		Application: "app",
		Environment: "prd",
		State:       string(StateOpen),
	}
	openItem, _ := attributevalue.MarshalMap(expectedOpen)
	mockErr := errors.New("dynamodb error")

	tests := []struct {
		name          string
		getItemFunc   func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error)
		expectedState string
		expectedError error
	}{
		{
			name: "Item not found returns Closed",
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: nil}, nil
			},
			expectedState: string(StateClosed),
			expectedError: nil,
		},
		{
			name: "GetItem returns correct state",
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: openItem}, nil
			},
			expectedState: string(StateOpen),
			expectedError: nil,
		},
		{
			name: "GetItem fails",
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return nil, mockErr
			},
			expectedState: "",
			expectedError: mockErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 100*time.Millisecond)
			scb.client = &mockDynamoDB{getItemFunc: tc.getItemFunc}

			stateStr, err := scb.GetState(context.Background(), "app", "prd")
			if tc.expectedError != nil {
				if !errors.Is(err, tc.expectedError) {
					t.Errorf("expected error %v, got %v", tc.expectedError, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if stateStr != tc.expectedState {
					t.Errorf("expected state %v, got %v", tc.expectedState, stateStr)
				}
			}
		})
	}
}

func TestSharedCircuitBreaker_Call(t *testing.T) {
	t.Run("Successful call Closed state", func(t *testing.T) {
		var putCalled bool
		mockClient := &mockDynamoDB{
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: nil}, nil // Returns closed
			},
			putItemFunc: func(ctx context.Context, params *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
				putCalled = true
				return &dynamodb.PutItemOutput{}, nil
			},
		}

		scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 100*time.Millisecond)
		scb.client = mockClient

		called := false
		err := scb.Call(context.Background(), "app", "prd", func() error {
			called = true
			return nil
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Error("expected inner function to be called")
		}
		if !putCalled {
			t.Error("expected putItem to be called to save success state")
		}
	})

	t.Run("DynamoDB fails falls back to local circuit breaker", func(t *testing.T) {
		mockClient := &mockDynamoDB{
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return nil, errors.New("dynamo connection fail")
			},
		}

		scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 2, 50*time.Millisecond)
		scb.client = mockClient

		errDummy := errors.New("dummy error")

		// Trigger failures on local fallback
		_ = scb.Call(context.Background(), "app", "prd", func() error { return errDummy })
		_ = scb.Call(context.Background(), "app", "prd", func() error { return errDummy })

		// Next call should fail with ErrCircuitOpen from local fallback
		err := scb.Call(context.Background(), "app", "prd", func() error { return nil })
		if !errors.Is(err, ErrCircuitOpen) {
			t.Errorf("expected ErrCircuitOpen, got %v", err)
		}
	})

	t.Run("State Open fails fast with ErrCircuitOpen", func(t *testing.T) {
		dbState := circuitState{
			Application:     "app",
			Environment:     "prd",
			State:           string(StateOpen),
			LastFailureTime: time.Now().UnixMilli(),
		}
		item, _ := attributevalue.MarshalMap(dbState)

		mockClient := &mockDynamoDB{
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: item}, nil
			},
		}

		scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 10*time.Second)
		scb.client = mockClient

		err := scb.Call(context.Background(), "app", "prd", func() error { return nil })
		if !errors.Is(err, ErrCircuitOpen) {
			t.Errorf("expected ErrCircuitOpen, got %v", err)
		}
	})

	t.Run("Transition Open -> Half-Open -> Closed on success", func(t *testing.T) {
		// Open state that expired
		dbState := circuitState{
			Application:     "app",
			Environment:     "prd",
			State:           string(StateOpen),
			LastFailureTime: time.Now().Add(-10 * time.Second).UnixMilli(),
		}
		item, _ := attributevalue.MarshalMap(dbState)

		var putState circuitState
		mockClient := &mockDynamoDB{
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: item}, nil
			},
			putItemFunc: func(ctx context.Context, params *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
				_ = attributevalue.UnmarshalMap(params.Item, &putState)
				return &dynamodb.PutItemOutput{}, nil
			},
		}

		scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 50*time.Millisecond)
		scb.client = mockClient

		err := scb.Call(context.Background(), "app", "prd", func() error { return nil })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if putState.State != string(StateClosed) {
			t.Errorf("expected DynamoDB state to transition to Closed, got %v", putState.State)
		}
	})

	t.Run("Transition Open -> Half-Open -> Open on failure", func(t *testing.T) {
		// Open state that expired
		dbState := circuitState{
			Application:     "app",
			Environment:     "prd",
			State:           string(StateOpen),
			LastFailureTime: time.Now().Add(-10 * time.Second).UnixMilli(),
		}
		item, _ := attributevalue.MarshalMap(dbState)

		var putState circuitState
		mockClient := &mockDynamoDB{
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: item}, nil
			},
			putItemFunc: func(ctx context.Context, params *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
				_ = attributevalue.UnmarshalMap(params.Item, &putState)
				return &dynamodb.PutItemOutput{}, nil
			},
		}

		scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 50*time.Millisecond)
		scb.client = mockClient

		errDummy := errors.New("dummy error")
		err := scb.Call(context.Background(), "app", "prd", func() error { return errDummy })
		if !errors.Is(err, errDummy) {
			t.Fatalf("expected dummy error, got %v", err)
		}

		if putState.State != string(StateOpen) {
			t.Errorf("expected DynamoDB state to transition back to Open on failure, got %v", putState.State)
		}
	})

	t.Run("getState UnmarshalMap failure", func(t *testing.T) {
		// Return invalid item attributes (e.g. failureCount is a List)
		invalidItem := map[string]types.AttributeValue{
			"PK":           &types.AttributeValueMemberS{Value: "app"},
			"SK":           &types.AttributeValueMemberS{Value: "prd"},
			"failureCount": &types.AttributeValueMemberL{Value: []types.AttributeValue{}},
		}

		mockClient := &mockDynamoDB{
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: invalidItem}, nil
			},
		}

		scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 50*time.Millisecond)
		scb.client = mockClient

		_, err := scb.getState(context.Background(), "app", "prd")
		if err == nil {
			t.Error("expected unmarshal error, got nil")
		}
	})

	t.Run("recordDynamoSuccess resets error state", func(t *testing.T) {
		mockClient := &mockDynamoDB{
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: nil}, nil
			},
			putItemFunc: func(ctx context.Context, params *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
				return &dynamodb.PutItemOutput{}, nil
			},
		}

		scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 50*time.Millisecond)
		scb.client = mockClient

		// Artificially trigger a Dynamo error
		scb.recordDynamoError()
		if !scb.shouldUseFallback() {
			t.Fatal("expected fallback to be active after error")
		}

		// Artificially expire the dynamo retry cooldown (so shouldUseFallback becomes false, but lastDynamoError is still not zero)
		scb.mu.Lock()
		scb.lastDynamoError = time.Now().Add(-10 * time.Second)
		scb.mu.Unlock()

		if scb.shouldUseFallback() {
			t.Fatal("expected fallback to be inactive after cooldown expired")
		}

		// Now make a call that succeeds (this calls setState and recordDynamoSuccess)
		err := scb.Call(context.Background(), "app", "prd", func() error { return nil })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// lastDynamoError should be reset to zero
		scb.mu.Lock()
		isZero := scb.lastDynamoError.IsZero()
		scb.mu.Unlock()

		if !isZero {
			t.Error("expected lastDynamoError to be reset to zero after successful DynamoDB call")
		}
	})
}

func TestSharedCircuitBreaker_ListStates(t *testing.T) {
	states := []circuitState{
		{Application: "app", Environment: "prd", State: "closed"},
		{Application: "app", Environment: "stg", State: "open"},
	}
	items := make([]map[string]types.AttributeValue, len(states))
	for i, s := range states {
		items[i], _ = attributevalue.MarshalMap(s)
	}
	mockErr := errors.New("dynamo query failed")

	invalidItem := map[string]types.AttributeValue{
		"PK":           &types.AttributeValueMemberS{Value: "app"},
		"failureCount": &types.AttributeValueMemberL{Value: []types.AttributeValue{}},
	}

	tests := []struct {
		name          string
		queryFunc     func(ctx context.Context, params *dynamodb.QueryInput) (*dynamodb.QueryOutput, error)
		expectedLen   int
		expectedError error
	}{
		{
			name: "Query succeeds",
			queryFunc: func(ctx context.Context, params *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
				return &dynamodb.QueryOutput{Items: items}, nil
			},
			expectedLen:   2,
			expectedError: nil,
		},
		{
			name: "Query fails",
			queryFunc: func(ctx context.Context, params *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
				return nil, mockErr
			},
			expectedLen:   0,
			expectedError: mockErr,
		},
		{
			name: "UnmarshalListOfMaps fails",
			queryFunc: func(ctx context.Context, params *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
				return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{invalidItem}}, nil
			},
			expectedLen:   0,
			expectedError: mockErr, // check presence of error
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 100*time.Millisecond)
			scb.client = &mockDynamoDB{queryFunc: tc.queryFunc}

			results, err := scb.ListStates(context.Background(), "app")
			if tc.expectedError != nil {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(results) != tc.expectedLen {
					t.Errorf("expected %d states, got %d", tc.expectedLen, len(results))
				}
			}
		})
	}
}

func TestSharedCircuitBreaker_SetStateMarshalError(t *testing.T) {
	mockErr := errors.New("marshal error")
	origMarshalMap := marshalMap
	defer func() { marshalMap = origMarshalMap }()

	marshalMap = func(v interface{}) (map[string]types.AttributeValue, error) {
		return nil, mockErr
	}

	scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 100*time.Millisecond)
	err := scb.setState(context.Background(), &circuitState{})
	if !errors.Is(err, mockErr) {
		t.Errorf("expected error %v, got %v", mockErr, err)
	}
}
