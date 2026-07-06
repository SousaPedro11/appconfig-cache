package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sousapedro11/appconfig-cache/internal/application"
	"github.com/sousapedro11/appconfig-cache/internal/bootstrap"
	"github.com/sousapedro11/appconfig-cache/internal/domain"
	"github.com/sousapedro11/appconfig-cache/internal/transport/auth"
	"github.com/sousapedro11/appconfig-cache/internal/transport/configpayload"
)

type errorBody struct {
	Message string `json:"message"`
}

const (
	defaultHTTPAddr = ":8080"
	contentTypeJSON = "application/json"
)

var notifyContext = signal.NotifyContext
var osExit = os.Exit
var shutdownTimeout = 10 * time.Second
var shutdownServer = func(srv *http.Server, ctx context.Context) error {
	return srv.Shutdown(ctx)
}

func main() {
	ctx, stop := notifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Printf("failed to run server: %v\n", err)
		osExit(1)
	}
}

var buildRuntime = bootstrap.BuildRuntime

func run(ctx context.Context) error {
	runtime, err := buildRuntime(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := runtime.Close(); closeErr != nil {
			fmt.Printf("failed to close runtime: %v\n", closeErr)
		}
	}()
	apiKeyValidator := auth.NewAPIKeyValidator(os.Getenv("X_API_KEY"))
	handler := buildRouter(runtime, apiKeyValidator)

	address := envOr("HTTP_ADDR", defaultHTTPAddr)
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		fmt.Printf("server listening on %s\n", address)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("server error: %v\n", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := shutdownServer(server, shutdownCtx); err != nil {
		return err
	}
	return nil
}

func buildRouter(runtime bootstrap.Runtime, apiKeyValidator auth.APIKeyValidator) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/config", withAPIKey(apiKeyValidator, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not supported"))
			return
		}

		requestPayload, err := readPayload(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		configuration, err := runtime.GetConfiguration.Handle(r.Context(), application.GetConfigurationCommand{
			Application: requestPayload.Application,
			Environment: requestPayload.Environment,
			Profile:     requestPayload.Profile,
		})
		if err != nil {
			if isBadRequestError(err) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeConfiguration(w, http.StatusOK, configuration)
	}))
	return mux
}

func readPayload(r *http.Request) (configpayload.Payload, error) {
	result := configpayload.Payload{
		Application: r.URL.Query().Get("application"),
		Environment: r.URL.Query().Get("environment"),
		Profile:     r.URL.Query().Get("profile"),
	}

	if r.Method == http.MethodPost && r.Body != nil {
		defer r.Body.Close()
		if body, err := io.ReadAll(r.Body); err == nil {
			if bodyPayload, err := configpayload.ParseJSON(body); err == nil {
				result.MergeMissing(bodyPayload)
			}
		}
	}

	if err := result.Validate(); err != nil {
		return configpayload.Payload{}, err
	}

	return result, nil
}

func writeJSON(w http.ResponseWriter, statusCode int, value interface{}) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, statusCode int, err error) {
	writeJSON(w, statusCode, errorBody{Message: err.Error()})
}

func isBadRequestError(err error) bool {
	return errors.Is(err, configpayload.ErrMissingFields) || errors.Is(err, domain.ErrInvalidConfigurationRequest)
}

func writeConfiguration(w http.ResponseWriter, statusCode int, configuration string) {
	if json.Valid([]byte(configuration)) {
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(configuration))
		return
	}

	writeJSON(w, statusCode, map[string]string{"configuration": configuration})
}

func withAPIKey(validator auth.APIKeyValidator, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := validator.Validate(r.Header.Get(auth.HeaderName), r.URL.Query().Get(auth.QueryKey)); err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}

		next(w, r)
	}
}

func envOr(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
