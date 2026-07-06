package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/sousapedro11/appconfig-cache/internal/application"
	"github.com/sousapedro11/appconfig-cache/internal/bootstrap"
	"github.com/sousapedro11/appconfig-cache/internal/domain"
	"github.com/sousapedro11/appconfig-cache/internal/transport/auth"
	"github.com/sousapedro11/appconfig-cache/internal/transport/configpayload"
)

// ponytail: simple in-memory stubs instead of launching real Valkey/AWS containers or complex network listeners, keeping tests 100% hermetic and instant.
type mockSource struct {
	getLatestFunc func(ctx context.Context, app domain.ApplicationID, env domain.EnvironmentID, prof domain.ProfileID) (domain.Configuration, error)
}

func (m *mockSource) GetLatestConfiguration(ctx context.Context, app domain.ApplicationID, env domain.EnvironmentID, prof domain.ProfileID) (domain.Configuration, error) {
	return m.getLatestFunc(ctx, app, env, prof)
}

type mockL1 struct {
	getFunc func(key domain.CacheKey) (domain.Configuration, bool)
	setFunc func(key domain.CacheKey, value domain.Configuration)
}

func (m *mockL1) Get(key domain.CacheKey) (domain.Configuration, bool) {
	return m.getFunc(key)
}

func (m *mockL1) Set(key domain.CacheKey, value domain.Configuration) {
	m.setFunc(key, value)
}

type mockL2 struct {
	getFunc func(ctx context.Context, key domain.CacheKey) (domain.Configuration, bool, error)
	setFunc func(ctx context.Context, key domain.CacheKey, value domain.Configuration) error
}

func (m *mockL2) Get(ctx context.Context, key domain.CacheKey) (domain.Configuration, bool, error) {
	return m.getFunc(ctx, key)
}

func (m *mockL2) Set(ctx context.Context, key domain.CacheKey, value domain.Configuration) error {
	return m.setFunc(ctx, key, value)
}

func TestLambdaHandler(t *testing.T) {
	// Setup standard mock dependencies
	src := &mockSource{
		getLatestFunc: func(ctx context.Context, app domain.ApplicationID, env domain.EnvironmentID, prof domain.ProfileID) (domain.Configuration, error) {
			return domain.NewConfiguration("{\"key\":\"val\"}")
		},
	}
	l1 := &mockL1{
		getFunc: func(key domain.CacheKey) (domain.Configuration, bool) {
			return domain.Configuration{}, false
		},
		setFunc: func(key domain.CacheKey, value domain.Configuration) {},
	}
	l2 := &mockL2{
		getFunc: func(ctx context.Context, key domain.CacheKey) (domain.Configuration, bool, error) {
			return domain.Configuration{}, false, nil
		},
		setFunc: func(ctx context.Context, key domain.CacheKey, value domain.Configuration) error {
			return nil
		},
	}

	service := application.NewCacheAsideService(src, l1, l2)
	getConfigurationMock := application.NewGetConfigurationHandler(service)

	setupMockRuntime := func() {
		resetRuntimeState()
		buildRuntime = func(ctx context.Context) (bootstrap.Runtime, error) {
			return bootstrap.Runtime{
				Service:          service,
				GetConfiguration: getConfigurationMock,
			}, nil
		}
	}

	t.Run("Runtime Initialization Failure", func(t *testing.T) {
		resetRuntimeState()
		buildRuntime = func(ctx context.Context) (bootstrap.Runtime, error) {
			return bootstrap.Runtime{}, errors.New("mock initialization error")
		}

		resp, err := handler(context.Background(), events.APIGatewayProxyRequest{})
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected 500 status, got %d", resp.StatusCode)
		}
	})

	t.Run("Authorization Failure - Missing Token", func(t *testing.T) {
		setupMockRuntime()
		_ = os.Setenv("X_API_KEY", "secret-token")
		defer os.Unsetenv("X_API_KEY")

		req := events.APIGatewayProxyRequest{
			Headers: map[string]string{},
		}

		resp, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 status, got %d", resp.StatusCode)
		}
	})

	t.Run("Authorization Failure - Wrong Token", func(t *testing.T) {
		setupMockRuntime()
		_ = os.Setenv("X_API_KEY", "secret-token")
		defer os.Unsetenv("X_API_KEY")

		req := events.APIGatewayProxyRequest{
			Headers: map[string]string{
				auth.HeaderName: "wrong-token",
			},
		}

		resp, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 status, got %d", resp.StatusCode)
		}
	})

	t.Run("Payload Validation Failure - Missing Fields", func(t *testing.T) {
		setupMockRuntime()
		_ = os.Setenv("X_API_KEY", "secret-token")
		defer os.Unsetenv("X_API_KEY")

		req := events.APIGatewayProxyRequest{
			Headers: map[string]string{
				auth.HeaderName: "secret-token",
			},
			QueryStringParameters: map[string]string{
				"application": "test-app",
			},
		}

		resp, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 status, got %d", resp.StatusCode)
		}
	})

	t.Run("Query Params Success Response", func(t *testing.T) {
		setupMockRuntime()
		_ = os.Setenv("X_API_KEY", "secret-token")
		defer os.Unsetenv("X_API_KEY")

		req := events.APIGatewayProxyRequest{
			Headers: map[string]string{
				auth.HeaderName: "secret-token",
			},
			QueryStringParameters: map[string]string{
				"application": "test-app",
				"environment": "dev",
				"profile":     "main",
			},
		}

		resp, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 status, got %d", resp.StatusCode)
		}
		if resp.Body != "{\"key\":\"val\"}" {
			t.Errorf("expected body to contain configuration, got %q", resp.Body)
		}
	})

	t.Run("Path Parameters Merge Success", func(t *testing.T) {
		setupMockRuntime()
		_ = os.Setenv("X_API_KEY", "secret-token")
		defer os.Unsetenv("X_API_KEY")

		req := events.APIGatewayProxyRequest{
			Headers: map[string]string{
				auth.HeaderName: "secret-token",
			},
			PathParameters: map[string]string{
				"application": "test-app",
				"environment": "dev",
				"profile":     "main",
			},
		}

		resp, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 status, got %d", resp.StatusCode)
		}
	})

	t.Run("JSON Body Merge Success", func(t *testing.T) {
		setupMockRuntime()
		_ = os.Setenv("X_API_KEY", "secret-token")
		defer os.Unsetenv("X_API_KEY")

		req := events.APIGatewayProxyRequest{
			Headers: map[string]string{
				auth.HeaderName: "secret-token",
			},
			Body: `{"application":"test-app","environment":"dev","profile":"main"}`,
		}

		resp, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 status, got %d", resp.StatusCode)
		}
	})

	t.Run("JSON Body Invalid is Ignored but Validates", func(t *testing.T) {
		setupMockRuntime()
		_ = os.Setenv("X_API_KEY", "secret-token")
		defer os.Unsetenv("X_API_KEY")

		req := events.APIGatewayProxyRequest{
			Headers: map[string]string{
				auth.HeaderName: "secret-token",
			},
			QueryStringParameters: map[string]string{
				"application": "test-app",
				"environment": "dev",
				"profile":     "main",
			},
			Body: `{invalid-json}`,
		}

		resp, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 status, got %d", resp.StatusCode)
		}
	})

	t.Run("Handler Domain Validation Error Map", func(t *testing.T) {
		setupMockRuntime()
		_ = os.Setenv("X_API_KEY", "secret-token")
		defer os.Unsetenv("X_API_KEY")

		// Query param is space string, triggers domain invalidation inside Handle
		req := events.APIGatewayProxyRequest{
			Headers: map[string]string{
				auth.HeaderName: "secret-token",
			},
			QueryStringParameters: map[string]string{
				"application": " ",
				"environment": "dev",
				"profile":     "main",
			},
		}

		resp, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 status, got %d", resp.StatusCode)
		}
	})

	t.Run("Handler Internal Service Error Map", func(t *testing.T) {
		// Mock source database breakdown
		srcErr := &mockSource{
			getLatestFunc: func(ctx context.Context, app domain.ApplicationID, env domain.EnvironmentID, prof domain.ProfileID) (domain.Configuration, error) {
				return domain.Configuration{}, errors.New("secrets lookup failure")
			},
		}
		errService := application.NewCacheAsideService(srcErr, l1, l2)
		errGetConfiguration := application.NewGetConfigurationHandler(errService)

		resetRuntimeState()
		buildRuntime = func(ctx context.Context) (bootstrap.Runtime, error) {
			return bootstrap.Runtime{
				Service:          errService,
				GetConfiguration: errGetConfiguration,
			}, nil
		}

		_ = os.Setenv("X_API_KEY", "secret-token")
		defer os.Unsetenv("X_API_KEY")

		req := events.APIGatewayProxyRequest{
			Headers: map[string]string{
				auth.HeaderName: "secret-token",
			},
			QueryStringParameters: map[string]string{
				"application": "test-app",
				"environment": "dev",
				"profile":     "main",
			},
		}

		resp, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected 500 status, got %d", resp.StatusCode)
		}
	})

	t.Run("Plain Text Configuration Response Encapsulation", func(t *testing.T) {
		srcText := &mockSource{
			getLatestFunc: func(ctx context.Context, app domain.ApplicationID, env domain.EnvironmentID, prof domain.ProfileID) (domain.Configuration, error) {
				return domain.NewConfiguration("plain-text-config-content")
			},
		}
		textService := application.NewCacheAsideService(srcText, l1, l2)
		textGetConfiguration := application.NewGetConfigurationHandler(textService)

		resetRuntimeState()
		buildRuntime = func(ctx context.Context) (bootstrap.Runtime, error) {
			return bootstrap.Runtime{
				Service:          textService,
				GetConfiguration: textGetConfiguration,
			}, nil
		}

		_ = os.Setenv("X_API_KEY", "secret-token")
		defer os.Unsetenv("X_API_KEY")

		req := events.APIGatewayProxyRequest{
			Headers: map[string]string{
				auth.HeaderName: "secret-token",
			},
			QueryStringParameters: map[string]string{
				"application": "test-app",
				"environment": "dev",
				"profile":     "main",
			},
		}

		resp, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 status, got %d", resp.StatusCode)
		}
		expectedBody := `{"configuration":"plain-text-config-content"}`
		if resp.Body != expectedBody {
			t.Errorf("expected wrapped text response %q, got %q", expectedBody, resp.Body)
		}
	})

	t.Run("Success Response Formatting Failure", func(t *testing.T) {
		setupMockRuntime()
		_ = os.Setenv("X_API_KEY", "secret-token")
		defer os.Unsetenv("X_API_KEY")

		origSuccess := successBodyFn
		successBodyFn = func(configuration string) ([]byte, error) {
			return nil, errors.New("mock formatting error")
		}
		defer func() { successBodyFn = origSuccess }()

		req := events.APIGatewayProxyRequest{
			Headers: map[string]string{
				auth.HeaderName: "secret-token",
			},
			QueryStringParameters: map[string]string{
				"application": "test-app",
				"environment": "dev",
				"profile":     "main",
			},
		}

		resp, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected 500 status, got %d", resp.StatusCode)
		}
	})
}

func TestIsBadRequestErrorLambda(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "missing fields", err: configpayload.ErrMissingFields, want: true},
		{name: "domain invalid", err: domain.ErrInvalidConfigurationRequest, want: true},
		{name: "wrapped missing fields", err: fmt.Errorf("wrapped: %w", configpayload.ErrMissingFields), want: true},
		{name: "generic error", err: errors.New("boom"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBadRequestError(tt.err)
			if got != tt.want {
				t.Fatalf("resultado inesperado: got=%v want=%v err=%v", got, tt.want, tt.err)
			}
		})
	}
}

func TestMain(t *testing.T) {
	origStart := lambdaStart
	lambdaStart = func(h interface{}) {}
	defer func() { lambdaStart = origStart }()

	main()
}
