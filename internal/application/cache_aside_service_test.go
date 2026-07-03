package application

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

func TestCacheAsideService_Get(t *testing.T) {
	expectedConfig, _ := domain.NewConfiguration(`{"key": "val"}`)
	mockErr := errors.New("domain validation error")

	tests := []struct {
		name          string
		app           string
		env           string
		prof          string
		sourceFunc    func(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error)
		expectedVal   string
		expectedError error
	}{
		{
			name: "Success path",
			app:  "app",
			env:  "prd",
			prof: "default",
			sourceFunc: func(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error) {
				return expectedConfig, nil
			},
			expectedVal:   `{"key": "val"}`,
			expectedError: nil,
		},
		{
			name:          "Validation error",
			app:           "",
			env:           "prd",
			prof:          "default",
			expectedVal:   "",
			expectedError: mockErr, // check presence of validation error
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := &mockConfigurationSource{getLatestConfigurationFunc: tc.sourceFunc}
			l1 := &mockL1Cache{}
			l2 := &mockL2Cache{}
			service := NewCacheAsideService(src, l1, l2)

			config, err := service.Get(context.Background(), tc.app, tc.env, tc.prof)
			if tc.expectedError != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if config.Content() != tc.expectedVal {
					t.Errorf("expected config content %q, got %q", tc.expectedVal, config.Content())
				}
			}
		})
	}
}

func TestCacheAsideService_GetByRequest(t *testing.T) {
	expectedConfig, _ := domain.NewConfiguration(`{"key": "value"}`)
	mockErr := errors.New("source error")
	request, _ := domain.NewConfigurationRequest("app", "prd", "default")

	tests := []struct {
		name          string
		setupCaches   func(l1 *mockL1Cache, l2 *mockL2Cache)
		sourceFunc    func(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error)
		expectedVal   string
		expectedError error
	}{
		{
			name: "L1 Cache Hit",
			setupCaches: func(l1 *mockL1Cache, l2 *mockL2Cache) {
				l1.getFunc = func(k domain.CacheKey) (domain.Configuration, bool) {
					return expectedConfig, true
				}
			},
			expectedVal:   `{"key": "value"}`,
			expectedError: nil,
		},
		{
			name: "L1 Miss, L2 Hit (Hydrates L1)",
			setupCaches: func(l1 *mockL1Cache, l2 *mockL2Cache) {
				l2.getFunc = func(ctx context.Context, k domain.CacheKey) (domain.Configuration, bool, error) {
					return expectedConfig, true, nil
				}
				l1.setFunc = func(k domain.CacheKey, val domain.Configuration) {
				}
			},
			expectedVal:   `{"key": "value"}`,
			expectedError: nil,
		},
		{
			name: "L1 Miss, L2 Miss, L3 Hit (Hydrates L1 and L2)",
			setupCaches: func(l1 *mockL1Cache, l2 *mockL2Cache) {
				l2.getFunc = func(ctx context.Context, k domain.CacheKey) (domain.Configuration, bool, error) {
					return domain.Configuration{}, false, nil
				}
			},
			sourceFunc: func(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error) {
				return expectedConfig, nil
			},
			expectedVal:   `{"key": "value"}`,
			expectedError: nil,
		},
		{
			name: "L1 Miss, L2 Miss, L3 Failure",
			setupCaches: func(l1 *mockL1Cache, l2 *mockL2Cache) {
				l2.getFunc = func(ctx context.Context, k domain.CacheKey) (domain.Configuration, bool, error) {
					return domain.Configuration{}, false, nil
				}
			},
			sourceFunc: func(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error) {
				return domain.Configuration{}, mockErr
			},
			expectedVal:   "",
			expectedError: mockErr,
		},
		{
			name: "L1 Miss, L2 Error, L3 Hit (Tolerates L2 Get Error)",
			setupCaches: func(l1 *mockL1Cache, l2 *mockL2Cache) {
				l2.getFunc = func(ctx context.Context, k domain.CacheKey) (domain.Configuration, bool, error) {
					return domain.Configuration{}, false, errors.New("redis timeout")
				}
			},
			sourceFunc: func(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error) {
				return expectedConfig, nil
			},
			expectedVal:   `{"key": "value"}`,
			expectedError: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := &mockConfigurationSource{getLatestConfigurationFunc: tc.sourceFunc}
			l1 := &mockL1Cache{}
			l2 := &mockL2Cache{}
			if tc.setupCaches != nil {
				tc.setupCaches(l1, l2)
			}

			service := NewCacheAsideService(src, l1, l2)
			config, err := service.GetByRequest(context.Background(), request)

			if tc.expectedError != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, tc.expectedError) {
					t.Errorf("expected error %v, got %v", tc.expectedError, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if config.Content() != tc.expectedVal {
					t.Errorf("expected config content %q, got %q", tc.expectedVal, config.Content())
				}
			}
		})
	}
}

func TestCacheAsideService_Concurrency_Singleflight(t *testing.T) {
	expectedConfig, _ := domain.NewConfiguration(`{"key": "concurrent"}`)
	request, _ := domain.NewConfigurationRequest("app", "prd", "default")

	var sourceCalls int32
	// Simulated slow L3 source
	src := &mockConfigurationSource{
		getLatestConfigurationFunc: func(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error) {
			atomic.AddInt32(&sourceCalls, 1)
			time.Sleep(50 * time.Millisecond) // Slow down fetch to ensure concurrency overlap
			return expectedConfig, nil
		},
	}

	l1 := &mockL1Cache{}
	// Simulated L2 cache miss
	l2 := &mockL2Cache{
		getFunc: func(ctx context.Context, key domain.CacheKey) (domain.Configuration, bool, error) {
			return domain.Configuration{}, false, nil
		},
	}

	service := NewCacheAsideService(src, l1, l2)

	// Spawn 10 concurrent requests
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	results := make([]domain.Configuration, numGoroutines)
	errorsList := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			config, err := service.GetByRequest(context.Background(), request)
			results[idx] = config
			errorsList[idx] = err
		}(i)
	}

	wg.Wait()

	// Assertions
	calls := atomic.LoadInt32(&sourceCalls)
	if calls != 1 {
		t.Errorf("expected exactly 1 call to L3 source, got %d (singleflight failed)", calls)
	}

	for i := 0; i < numGoroutines; i++ {
		if errorsList[i] != nil {
			t.Errorf("goroutine %d failed: %v", i, errorsList[i])
		}
		if results[i].Content() != "{\"key\": \"concurrent\"}" {
			t.Errorf("goroutine %d got unexpected content: %q", i, results[i].Content())
		}
	}
}
