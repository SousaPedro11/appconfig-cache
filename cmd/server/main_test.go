package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/sousapedro11/appconfig-cache/internal/application"
	"github.com/sousapedro11/appconfig-cache/internal/bootstrap"
	"github.com/sousapedro11/appconfig-cache/internal/domain"
	"github.com/sousapedro11/appconfig-cache/internal/transport/auth"
	"github.com/sousapedro11/appconfig-cache/internal/transport/configpayload"
)

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

func TestHTTPServer(t *testing.T) {
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
	handler := application.NewGetConfigurationHandler(service)
	runtime := bootstrap.Runtime{
		Service:          service,
		GetConfiguration: handler,
	}

	apiKeyVal := auth.NewAPIKeyValidator("test-api-key")
	router := buildRouter(runtime, apiKeyVal)

	t.Run("Health Endpoint Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
		if rec.Body.String() != "ok" {
			t.Errorf("expected body 'ok', got %q", rec.Body.String())
		}
	})

	t.Run("V1Config API Key Authorization Missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("V1Config Method Not Supported", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/v1/config", nil)
		req.Header.Set(auth.HeaderName, "test-api-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 Method Not Allowed, got %d", rec.Code)
		}
	})

	t.Run("V1Config Query Params Validation Failure", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/config?application=test", nil)
		req.Header.Set(auth.HeaderName, "test-api-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("V1Config Query Params Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/config?application=test-app&environment=dev&profile=main", nil)
		req.Header.Set(auth.HeaderName, "test-api-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
		if rec.Header().Get("Content-Type") != contentTypeJSON {
			t.Errorf("expected header %q, got %q", contentTypeJSON, rec.Header().Get("Content-Type"))
		}
		if rec.Body.String() != "{\"key\":\"val\"}" {
			t.Errorf("expected JSON content, got %q", rec.Body.String())
		}
	})

	t.Run("V1Config Post JSON Body Success", func(t *testing.T) {
		body := []byte(`{"application":"test-app","environment":"dev","profile":"main"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/config", bytes.NewReader(body))
		req.Header.Set(auth.HeaderName, "test-api-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("V1Config Non-JSON Configuration Success Mapping", func(t *testing.T) {
		srcText := &mockSource{
			getLatestFunc: func(ctx context.Context, app domain.ApplicationID, env domain.EnvironmentID, prof domain.ProfileID) (domain.Configuration, error) {
				return domain.NewConfiguration("plain-text-config-content")
			},
		}
		textService := application.NewCacheAsideService(srcText, l1, l2)
		textHandler := application.NewGetConfigurationHandler(textService)
		textRuntime := bootstrap.Runtime{
			Service:          textService,
			GetConfiguration: textHandler,
		}
		textRouter := buildRouter(textRuntime, apiKeyVal)

		req := httptest.NewRequest(http.MethodGet, "/v1/config?application=test-app&environment=dev&profile=main", nil)
		req.Header.Set(auth.HeaderName, "test-api-key")
		rec := httptest.NewRecorder()
		textRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
		expectedBody := `{"configuration":"plain-text-config-content"}` + "\n"
		if rec.Body.String() != expectedBody {
			t.Errorf("expected wrapped text response %q, got %q", expectedBody, rec.Body.String())
		}
	})

	t.Run("V1Config Domain ValidationError Mapping", func(t *testing.T) {
		body := []byte(`{"application":" ","environment":"dev","profile":"main"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/config", bytes.NewReader(body))
		req.Header.Set(auth.HeaderName, "test-api-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("V1Config Handler Internal Server Error Mapping", func(t *testing.T) {
		errMock := errors.New("database breakdown")
		srcErr := &mockSource{
			getLatestFunc: func(ctx context.Context, app domain.ApplicationID, env domain.EnvironmentID, prof domain.ProfileID) (domain.Configuration, error) {
				return domain.Configuration{}, errMock
			},
		}
		errService := application.NewCacheAsideService(srcErr, l1, l2)
		errHandler := application.NewGetConfigurationHandler(errService)
		errRuntime := bootstrap.Runtime{
			Service:          errService,
			GetConfiguration: errHandler,
		}
		errRouter := buildRouter(errRuntime, apiKeyVal)

		req := httptest.NewRequest(http.MethodGet, "/v1/config?application=test-app&environment=dev&profile=main", nil)
		req.Header.Set(auth.HeaderName, "test-api-key")
		rec := httptest.NewRecorder()
		errRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500 Internal Server Error, got %d", rec.Code)
		}
		expectedBody := `{"message":"database breakdown"}` + "\n"
		if rec.Body.String() != expectedBody {
			t.Errorf("expected JSON error body %q, got %q", expectedBody, rec.Body.String())
		}
	})
}

func TestIsBadRequestErrorServer(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "missing fields", err: configpayload.ErrMissingFields, want: true},
		{name: "domain invalid", err: domain.ErrInvalidConfigurationRequest, want: true},
		{name: "wrapped domain invalid", err: fmt.Errorf("wrapped: %w", domain.ErrInvalidConfigurationRequest), want: true},
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

func TestEnvOr(t *testing.T) {
	os.Clearenv()
	_ = os.Setenv("TEST_KEY", "test-val")

	if got := envOr("TEST_KEY", "fallback"); got != "test-val" {
		t.Errorf("expected 'test-val', got %q", got)
	}

	if got := envOr("NON_EXISTENT_KEY", "fallback"); got != "fallback" {
		t.Errorf("expected 'fallback', got %q", got)
	}
}

func TestRunServer(t *testing.T) {
	t.Run("Happy Path", func(t *testing.T) {
		os.Clearenv()
		_ = os.Setenv("AWS_REGION", "us-west-2")
		_ = os.Setenv("HTTP_ADDR", "127.0.0.1:0")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := run(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("BuildRuntime Error", func(t *testing.T) {
		origBuild := buildRuntime
		buildRuntime = func(ctx context.Context) (bootstrap.Runtime, error) {
			return bootstrap.Runtime{}, errors.New("mock build error")
		}
		defer func() { buildRuntime = origBuild }()

		ctx := context.Background()
		err := run(ctx)
		if err == nil || err.Error() != "mock build error" {
			t.Errorf("expected mock build error, got %v", err)
		}
	})

	t.Run("Runtime Close Error", func(t *testing.T) {
		origBuild := buildRuntime
		buildRuntime = func(ctx context.Context) (bootstrap.Runtime, error) {
			return bootstrap.Runtime{
				Close: func() error {
					return errors.New("mock close error")
				},
			}, nil
		}
		defer func() { buildRuntime = origBuild }()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := run(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ListenAndServe Error", func(t *testing.T) {
		os.Clearenv()
		_ = os.Setenv("AWS_REGION", "us-west-2")
		// Use an invalid port to trigger ListenAndServe error instantly
		_ = os.Setenv("HTTP_ADDR", "127.0.0.1:99999")

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		err := run(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Shutdown Error", func(t *testing.T) {
		os.Clearenv()
		_ = os.Setenv("AWS_REGION", "us-west-2")
		_ = os.Setenv("HTTP_ADDR", "127.0.0.1:0")

		origShutdown := shutdownServer
		shutdownServer = func(srv *http.Server, ctx context.Context) error {
			return errors.New("mock shutdown error")
		}
		defer func() { shutdownServer = origShutdown }()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := run(ctx)
		if err == nil || err.Error() != "mock shutdown error" {
			t.Errorf("expected mock shutdown error, got %v", err)
		}
	})
}

func TestMain_Success(t *testing.T) {
	os.Clearenv()
	_ = os.Setenv("AWS_REGION", "us-west-2")
	_ = os.Setenv("HTTP_ADDR", "127.0.0.1:0")

	origNotify := notifyContext
	notifyContext = func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(parent)
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		return ctx, func() {}
	}
	defer func() { notifyContext = origNotify }()

	main()
}

func TestMain_Error(t *testing.T) {
	os.Clearenv()
	_ = os.Setenv("AWS_REGION", "us-west-2")
	_ = os.Setenv("HTTP_ADDR", "127.0.0.1:0")

	origBuild := buildRuntime
	buildRuntime = func(ctx context.Context) (bootstrap.Runtime, error) {
		return bootstrap.Runtime{}, errors.New("mock build error")
	}
	defer func() { buildRuntime = origBuild }()

	var exitCode int
	origExit := osExit
	osExit = func(code int) {
		exitCode = code
	}
	defer func() { osExit = origExit }()

	main()

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}
