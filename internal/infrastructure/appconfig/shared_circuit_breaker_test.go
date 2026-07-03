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
	t.Run("Item not found returns Closed", func(t *testing.T) {
		mockClient := &mockDynamoDB{
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: nil}, nil
			},
		}

		scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 100*time.Millisecond)
		scb.client = mockClient

		stateStr, err := scb.GetState(context.Background(), "app", "prd")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stateStr != string(StateClosed) {
			t.Errorf("expected Closed state, got %v", stateStr)
		}
	})

	t.Run("GetItem returns correct state", func(t *testing.T) {
		expectedState := circuitState{
			Application: "app",
			Environment: "prd",
			State:       string(StateOpen),
		}
		item, _ := attributevalue.MarshalMap(expectedState)

		mockClient := &mockDynamoDB{
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: item}, nil
			},
		}

		scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 100*time.Millisecond)
		scb.client = mockClient

		stateStr, err := scb.GetState(context.Background(), "app", "prd")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stateStr != string(StateOpen) {
			t.Errorf("expected Open state, got %v", stateStr)
		}
	})

	t.Run("GetItem fails", func(t *testing.T) {
		mockErr := errors.New("dynamodb error")
		mockClient := &mockDynamoDB{
			getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return nil, mockErr
			},
		}

		scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 100*time.Millisecond)
		scb.client = mockClient

		_, err := scb.GetState(context.Background(), "app", "prd")
		if !errors.Is(err, mockErr) {
			t.Errorf("expected error %v, got %v", mockErr, err)
		}
	})
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
}

func TestSharedCircuitBreaker_ListStates(t *testing.T) {
	t.Run("Query succeeds", func(t *testing.T) {
		states := []circuitState{
			{Application: "app", Environment: "prd", State: "closed"},
			{Application: "app", Environment: "stg", State: "open"},
		}
		items := make([]map[string]types.AttributeValue, len(states))
		for i, s := range states {
			items[i], _ = attributevalue.MarshalMap(s)
		}

		mockClient := &mockDynamoDB{
			queryFunc: func(ctx context.Context, params *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
				return &dynamodb.QueryOutput{
					Items: items,
				}, nil
			},
		}

		scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 100*time.Millisecond)
		scb.client = mockClient

		results, err := scb.ListStates(context.Background(), "app")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(results) != 2 {
			t.Fatalf("expected 2 states, got %d", len(results))
		}

		if results[0].Environment != "prd" || results[1].Environment != "stg" {
			t.Errorf("unexpected query results: %+v", results)
		}
	})

	t.Run("Query fails", func(t *testing.T) {
		mockErr := errors.New("dynamo query failed")
		mockClient := &mockDynamoDB{
			queryFunc: func(ctx context.Context, params *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
				return nil, mockErr
			},
		}

		scb := NewSharedCircuitBreaker(aws.Config{}, "test-table", 3, 100*time.Millisecond)
		scb.client = mockClient

		_, err := scb.ListStates(context.Background(), "app")
		if !errors.Is(err, mockErr) {
			t.Errorf("expected error %v, got %v", mockErr, err)
		}
	})
}
