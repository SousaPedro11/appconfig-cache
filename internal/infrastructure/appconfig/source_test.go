package appconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/appconfigdata"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

type mockAppConfigDataAPI struct {
	startSessionFunc func(ctx context.Context, params *appconfigdata.StartConfigurationSessionInput) (*appconfigdata.StartConfigurationSessionOutput, error)
	getConfigFunc    func(ctx context.Context, params *appconfigdata.GetLatestConfigurationInput) (*appconfigdata.GetLatestConfigurationOutput, error)
}

func (m *mockAppConfigDataAPI) StartConfigurationSession(ctx context.Context, params *appconfigdata.StartConfigurationSessionInput, optFns ...func(*appconfigdata.Options)) (*appconfigdata.StartConfigurationSessionOutput, error) {
	return m.startSessionFunc(ctx, params)
}

func (m *mockAppConfigDataAPI) GetLatestConfiguration(ctx context.Context, params *appconfigdata.GetLatestConfigurationInput, optFns ...func(*appconfigdata.Options)) (*appconfigdata.GetLatestConfigurationOutput, error) {
	return m.getConfigFunc(ctx, params)
}

func TestNewSourceUsesDefaultRequestTimeout(t *testing.T) {
	source := NewSource(aws.Config{})
	if got, want := source.requestTimeout, defaultRequestTimeout; got != want {
		t.Fatalf("timeout padrão inesperado: got=%v want=%v", got, want)
	}
}

func TestWithRequestTimeoutOverridesTimeout(t *testing.T) {
	source := NewSource(aws.Config{})
	source.WithRequestTimeout(12 * time.Second)

	if got, want := source.requestTimeout, 12*time.Second; got != want {
		t.Fatalf("timeout inesperado: got=%v want=%v", got, want)
	}
}

func TestGetLatestConfiguration_Validation(t *testing.T) {
	source := NewSource(aws.Config{})
	ctx := context.Background()

	tests := []struct {
		name string
		app  string
		env  string
		prof string
	}{
		{"Empty Application", "", "prd", "default"},
		{"Empty Environment", "app", "", "default"},
		{"Empty Profile", "app", "prd", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := source.GetLatestConfiguration(ctx, domain.ApplicationID(tc.app), domain.EnvironmentID(tc.env), domain.ProfileID(tc.prof))
			if err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestGetLatestConfiguration_Success(t *testing.T) {
	mockClient := &mockAppConfigDataAPI{
		startSessionFunc: func(ctx context.Context, params *appconfigdata.StartConfigurationSessionInput) (*appconfigdata.StartConfigurationSessionOutput, error) {
			token := "initial-token"
			return &appconfigdata.StartConfigurationSessionOutput{
				InitialConfigurationToken: &token,
			}, nil
		},
		getConfigFunc: func(ctx context.Context, params *appconfigdata.GetLatestConfigurationInput) (*appconfigdata.GetLatestConfigurationOutput, error) {
			return &appconfigdata.GetLatestConfigurationOutput{
				Configuration: []byte(`{"key": "value"}`),
			}, nil
		},
	}

	source := NewSource(aws.Config{})
	source.client = mockClient

	config, err := source.GetLatestConfiguration(context.Background(), "app", "prd", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Content() != `{"key": "value"}` {
		t.Errorf("expected content '{\"key\": \"value\"}', got %q", config.Content())
	}
}

func TestGetLatestConfiguration_Errors(t *testing.T) {
	mockStartErr := errors.New("aws start session error")
	mockGetConfigErr := errors.New("aws get config error")
	tokenVal := "token"
	emptyToken := ""

	tests := []struct {
		name             string
		startSessionFunc func(ctx context.Context, params *appconfigdata.StartConfigurationSessionInput) (*appconfigdata.StartConfigurationSessionOutput, error)
		getConfigFunc    func(ctx context.Context, params *appconfigdata.GetLatestConfigurationInput) (*appconfigdata.GetLatestConfigurationOutput, error)
		expectedError    error
	}{
		{
			name: "StartConfigurationSession fails",
			startSessionFunc: func(ctx context.Context, params *appconfigdata.StartConfigurationSessionInput) (*appconfigdata.StartConfigurationSessionOutput, error) {
				return nil, mockStartErr
			},
			expectedError: mockStartErr,
		},
		{
			name: "Empty session token",
			startSessionFunc: func(ctx context.Context, params *appconfigdata.StartConfigurationSessionInput) (*appconfigdata.StartConfigurationSessionOutput, error) {
				return &appconfigdata.StartConfigurationSessionOutput{
					InitialConfigurationToken: &emptyToken,
				}, nil
			},
			expectedError: errors.New("empty AppConfig initial configuration token"),
		},
		{
			name: "GetLatestConfiguration fails",
			startSessionFunc: func(ctx context.Context, params *appconfigdata.StartConfigurationSessionInput) (*appconfigdata.StartConfigurationSessionOutput, error) {
				return &appconfigdata.StartConfigurationSessionOutput{
					InitialConfigurationToken: &tokenVal,
				}, nil
			},
			getConfigFunc: func(ctx context.Context, params *appconfigdata.GetLatestConfigurationInput) (*appconfigdata.GetLatestConfigurationOutput, error) {
				return nil, mockGetConfigErr
			},
			expectedError: mockGetConfigErr,
		},
		{
			name: "Empty configuration returned",
			startSessionFunc: func(ctx context.Context, params *appconfigdata.StartConfigurationSessionInput) (*appconfigdata.StartConfigurationSessionOutput, error) {
				return &appconfigdata.StartConfigurationSessionOutput{
					InitialConfigurationToken: &tokenVal,
				}, nil
			},
			getConfigFunc: func(ctx context.Context, params *appconfigdata.GetLatestConfigurationInput) (*appconfigdata.GetLatestConfigurationOutput, error) {
				return &appconfigdata.GetLatestConfigurationOutput{
					Configuration: []byte{},
				}, nil
			},
			expectedError: errors.New("configuration not found"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &mockAppConfigDataAPI{
				startSessionFunc: tc.startSessionFunc,
				getConfigFunc:    tc.getConfigFunc,
			}

			source := NewSource(aws.Config{})
			source.client = mockClient

			_, err := source.GetLatestConfiguration(context.Background(), "app", "prd", "default")
			if tc.expectedError != nil {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !errors.Is(err, tc.expectedError) && err.Error() != tc.expectedError.Error() {
					// Check if specific mock error matches or if message is returned
					if errors.Is(tc.expectedError, mockStartErr) || errors.Is(tc.expectedError, mockGetConfigErr) {
						t.Errorf("expected error %v, got %v", tc.expectedError, err)
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestGetLatestConfiguration_LocalCircuitBreakerTriggers(t *testing.T) {
	mockErr := errors.New("fetch failed")
	mockClient := &mockAppConfigDataAPI{
		startSessionFunc: func(ctx context.Context, params *appconfigdata.StartConfigurationSessionInput) (*appconfigdata.StartConfigurationSessionOutput, error) {
			return nil, mockErr
		},
	}

	source := NewSource(aws.Config{})
	source.circuitBreaker = NewCircuitBreaker(3, 100*time.Millisecond)
	source.client = mockClient

	// Trigger 3 failures
	for i := 0; i < 3; i++ {
		_, err := source.GetLatestConfiguration(context.Background(), "app", "prd", "default")
		if !errors.Is(err, mockErr) {
			t.Fatalf("expected mockErr, got %v", err)
		}
	}

	// 4th call should fail fast with ErrCircuitOpen
	_, err := source.GetLatestConfiguration(context.Background(), "app", "prd", "default")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestGetLatestConfiguration_SharedCircuitBreaker(t *testing.T) {
	// Set up a source with a shared circuit breaker (with dummy DynamoDB configuration)
	// Because DynamoDB is not running, calling it will fail.
	// It should fall back to using the local circuit breaker fallback inside SharedCircuitBreaker.
	mockClient := &mockAppConfigDataAPI{
		startSessionFunc: func(ctx context.Context, params *appconfigdata.StartConfigurationSessionInput) (*appconfigdata.StartConfigurationSessionOutput, error) {
			token := "token"
			return &appconfigdata.StartConfigurationSessionOutput{
				InitialConfigurationToken: &token,
			}, nil
		},
		getConfigFunc: func(ctx context.Context, params *appconfigdata.GetLatestConfigurationInput) (*appconfigdata.GetLatestConfigurationOutput, error) {
			return &appconfigdata.GetLatestConfigurationOutput{
				Configuration: []byte("val"),
			}, nil
		},
	}

	source := NewSource(aws.Config{})
	scb := NewSharedCircuitBreaker(aws.Config{}, "CB_TABLE", 3, 100*time.Millisecond)
	source.WithSharedCircuitBreaker(scb)
	source.client = mockClient

	config, err := source.GetLatestConfiguration(context.Background(), "app", "prd", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Content() != "val" {
		t.Errorf("expected content 'val', got %q", config.Content())
	}
}

func TestGetLatestConfiguration_SharedCircuitBreaker_Error(t *testing.T) {
	mockErr := errors.New("shared cb error")
	mockClient := &mockAppConfigDataAPI{
		startSessionFunc: func(ctx context.Context, params *appconfigdata.StartConfigurationSessionInput) (*appconfigdata.StartConfigurationSessionOutput, error) {
			return nil, mockErr
		},
	}

	source := NewSource(aws.Config{})
	scb := NewSharedCircuitBreaker(aws.Config{}, "CB_TABLE", 3, 100*time.Millisecond)
	source.WithSharedCircuitBreaker(scb)
	source.client = mockClient

	_, err := source.GetLatestConfiguration(context.Background(), "app", "prd", "default")
	if !errors.Is(err, mockErr) {
		t.Errorf("expected error %v, got %v", mockErr, err)
	}
}

func TestGetClient_DefaultInitialization(t *testing.T) {
	source := NewSource(aws.Config{})
	client := source.getClient()
	if client == nil {
		t.Error("expected client to be initialized by default, got nil")
	}
}
