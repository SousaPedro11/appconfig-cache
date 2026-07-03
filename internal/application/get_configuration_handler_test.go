package application

import (
	"context"
	"errors"
	"testing"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

type mockConfigurationSource struct {
	getLatestConfigurationFunc func(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error)
}

func (m *mockConfigurationSource) GetLatestConfiguration(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error) {
	return m.getLatestConfigurationFunc(ctx, application, environment, profile)
}

type mockL1Cache struct {
	getFunc func(key domain.CacheKey) (domain.Configuration, bool)
	setFunc func(key domain.CacheKey, value domain.Configuration)
}

func (m *mockL1Cache) Get(key domain.CacheKey) (domain.Configuration, bool) {
	if m.getFunc != nil {
		return m.getFunc(key)
	}
	return domain.Configuration{}, false
}

func (m *mockL1Cache) Set(key domain.CacheKey, value domain.Configuration) {
	if m.setFunc != nil {
		m.setFunc(key, value)
	}
}

type mockL2Cache struct {
	getFunc func(ctx context.Context, key domain.CacheKey) (domain.Configuration, bool, error)
	setFunc func(ctx context.Context, key domain.CacheKey, value domain.Configuration) error
}

func (m *mockL2Cache) Get(ctx context.Context, key domain.CacheKey) (domain.Configuration, bool, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, key)
	}
	return domain.Configuration{}, false, nil
}

func (m *mockL2Cache) Set(ctx context.Context, key domain.CacheKey, value domain.Configuration) error {
	if m.setFunc != nil {
		return m.setFunc(ctx, key, value)
	}
	return nil
}

func TestGetConfigurationHandler_Handle(t *testing.T) {
	mockErr := errors.New("service failure")
	expectedConfig, _ := domain.NewConfiguration(`{"key": "value"}`)

	tests := []struct {
		name          string
		command       GetConfigurationCommand
		sourceFunc    func(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error)
		expectedVal   string
		expectedError error
	}{
		{
			name: "Successful handling",
			command: GetConfigurationCommand{
				Application: "app",
				Environment: "prd",
				Profile:     "default",
			},
			sourceFunc: func(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error) {
				return expectedConfig, nil
			},
			expectedVal:   `{"key": "value"}`,
			expectedError: nil,
		},
		{
			name: "Domain validation error (empty fields)",
			command: GetConfigurationCommand{
				Application: "",
				Environment: "prd",
				Profile:     "default",
			},
			expectedVal:   "",
			expectedError: domain.ErrInvalidConfigurationRequest,
		},
		{
			name: "Service error propagated",
			command: GetConfigurationCommand{
				Application: "app",
				Environment: "prd",
				Profile:     "default",
			},
			sourceFunc: func(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error) {
				return domain.Configuration{}, mockErr
			},
			expectedVal:   "",
			expectedError: mockErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := &mockConfigurationSource{getLatestConfigurationFunc: tc.sourceFunc}
			l1 := &mockL1Cache{}
			l2 := &mockL2Cache{}
			service := NewCacheAsideService(src, l1, l2)
			handler := NewGetConfigurationHandler(service)

			val, err := handler.Handle(context.Background(), tc.command)
			if tc.expectedError != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tc.expectedError)
				}
				if !errors.Is(err, tc.expectedError) {
					t.Errorf("expected error %v, got %v", tc.expectedError, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if val != tc.expectedVal {
					t.Errorf("expected value %q, got %q", tc.expectedVal, val)
				}
			}
		})
	}
}
