package bootstrap

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/redis/go-redis/v9"
)

type mockHttpClient struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHttpClient) Do(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestBuildRuntime(t *testing.T) {
	t.Run("Success without L2 Cache", func(t *testing.T) {
		os.Clearenv()
		_ = os.Setenv("AWS_REGION", "us-west-2")

		ctx := context.Background()
		runtime, err := BuildRuntime(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if runtime.Service == nil {
			t.Error("expected Service to be initialized, got nil")
		}
		if runtime.GetConfiguration == nil {
			t.Error("expected GetConfiguration handler to be initialized, got nil")
		}
		if runtime.Close == nil {
			t.Error("expected Close function to be initialized, got nil")
		}

		if err := runtime.Close(); err != nil {
			t.Errorf("unexpected error on Close: %v", err)
		}
	})

	t.Run("AWS Config Loading Failure", func(t *testing.T) {
		os.Clearenv()
		mockErr := errors.New("load aws config failed")

		origLoadAWSConfig := loadAWSConfig
		loadAWSConfig = func(ctx context.Context, region string) (aws.Config, error) {
			return aws.Config{}, mockErr
		}
		defer func() { loadAWSConfig = origLoadAWSConfig }()

		ctx := context.Background()
		_, err := BuildRuntime(ctx)
		if !errors.Is(err, mockErr) {
			t.Errorf("expected error %v, got %v", mockErr, err)
		}
	})

	t.Run("With Shared Circuit Breaker", func(t *testing.T) {
		os.Clearenv()
		_ = os.Setenv("CIRCUIT_BREAKER_TABLE_NAME", "CB_TEST")

		ctx := context.Background()
		runtime, err := BuildRuntime(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_ = runtime.Close()
	})

	t.Run("With Invalid TTL Config Fallback", func(t *testing.T) {
		os.Clearenv()
		_ = os.Setenv("L1_TTL_SECONDS", "invalid-int")

		ctx := context.Background()
		runtime, err := BuildRuntime(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_ = runtime.Close()
	})
}

func TestBuildL2Cache(t *testing.T) {
	t.Run("Valkey Port Defined Without Host Warning", func(t *testing.T) {
		os.Clearenv()
		_ = os.Setenv("VALKEY_PORT", "6379")

		ctx := context.Background()
		runtime, err := BuildRuntime(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_ = runtime.Close()
	})

	t.Run("Valkey Connection Refused Fallback to L1", func(t *testing.T) {
		os.Clearenv()
		_ = os.Setenv("VALKEY_HOST", "127.0.0.1")
		_ = os.Setenv("VALKEY_PORT", "12345") // Unreachable local port

		ctx := context.Background()
		runtime, err := BuildRuntime(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_ = runtime.Close()
	})

	t.Run("Secrets Manager Network Load Failure Fallback to L1", func(t *testing.T) {
		os.Clearenv()
		_ = os.Setenv("CACHE_SECRET_NAME", "valkey-credentials-secret")

		origLoadAWSConfig := loadAWSConfig
		loadAWSConfig = func(ctx context.Context, region string) (aws.Config, error) {
			mockHTTP := &mockHttpClient{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					return nil, errors.New("aws secrets manager offline (mocked error)")
				},
			}
			return aws.Config{
				Region:     region,
				HTTPClient: mockHTTP,
			}, nil
		}
		defer func() { loadAWSConfig = origLoadAWSConfig }()

		ctx := context.Background()
		runtime, err := BuildRuntime(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_ = runtime.Close()
	})

	t.Run("Direct Connection Success with Valkey L2 Cache", func(t *testing.T) {
		os.Clearenv()
		_ = os.Setenv("VALKEY_HOST", "127.0.0.1")
		_ = os.Setenv("VALKEY_PORT", "6379")

		origPingRedis := pingRedis
		pingRedis = func(ctx context.Context, client *redis.Client) error {
			return nil
		}
		defer func() { pingRedis = origPingRedis }()

		ctx := context.Background()
		runtime, err := BuildRuntime(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := runtime.Close(); err != nil {
			t.Errorf("failed to close runtime: %v", err)
		}
	})

	t.Run("Secrets Manager Load Success with Valkey L2 Cache", func(t *testing.T) {
		os.Clearenv()
		_ = os.Setenv("CACHE_SECRET_NAME", "valkey-secret")

		origPingRedis := pingRedis
		pingRedis = func(ctx context.Context, client *redis.Client) error {
			return nil
		}
		defer func() { pingRedis = origPingRedis }()

		origLoadAWSConfig := loadAWSConfig
		loadAWSConfig = func(ctx context.Context, region string) (aws.Config, error) {
			mockHTTP := &mockHttpClient{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					body := `{"SecretString": "{\"host\":\"127.0.0.1\",\"port\":6379}"}`
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(strings.NewReader(body)),
						Header:     make(http.Header),
					}, nil
				},
			}
			return aws.Config{
				Region:     region,
				HTTPClient: mockHTTP,
			}, nil
		}
		defer func() { loadAWSConfig = origLoadAWSConfig }()

		ctx := context.Background()
		runtime, err := BuildRuntime(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_ = runtime.Close()
	})
}
