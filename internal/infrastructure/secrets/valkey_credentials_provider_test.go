package secrets

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type mockSecretsManager struct {
	getSecretValueFunc func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

func (m *mockSecretsManager) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	return m.getSecretValueFunc(ctx, params, optFns...)
}

func TestValkeyCredentialsProvider(t *testing.T) {
	ctx := context.Background()
	secretName := "my-valkey-secret"

	t.Run("Success - JSON with numeric port", func(t *testing.T) {
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				rawJSON := `{"host": "valkey.internal", "port": 6380}`
				return &secretsmanager.GetSecretValueOutput{
					SecretString: &rawJSON,
				}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		creds, err := p.Get(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if creds.Host != "valkey.internal" {
			t.Errorf("expected host 'valkey.internal', got %q", creds.Host)
		}
		if creds.Port != 6380 {
			t.Errorf("expected port 6380, got %d", creds.Port)
		}
	})

	t.Run("Success - JSON with string port", func(t *testing.T) {
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				rawJSON := `{"host": "valkey-string.internal", "port": "1234"}`
				return &secretsmanager.GetSecretValueOutput{
					SecretString: &rawJSON,
				}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		creds, err := p.Get(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if creds.Host != "valkey-string.internal" {
			t.Errorf("expected host 'valkey-string.internal', got %q", creds.Host)
		}
		if creds.Port != 1234 {
			t.Errorf("expected port 1234, got %d", creds.Port)
		}
	})

	t.Run("Success - JSON using binary payload", func(t *testing.T) {
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
					SecretBinary: []byte(`{"host": "valkey-bin.internal", "port": 6379}`),
				}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		creds, err := p.Get(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if creds.Host != "valkey-bin.internal" {
			t.Errorf("expected host 'valkey-bin.internal', got %q", creds.Host)
		}
		if creds.Port != 6379 {
			t.Errorf("expected port 6379, got %d", creds.Port)
		}
	})

	t.Run("Success - JSON with empty/missing port falls back to 6379", func(t *testing.T) {
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				rawJSON := `{"host": "valkey-fallback.internal"}`
				return &secretsmanager.GetSecretValueOutput{
					SecretString: &rawJSON,
				}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		creds, err := p.Get(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if creds.Host != "valkey-fallback.internal" {
			t.Errorf("expected host 'valkey-fallback.internal', got %q", creds.Host)
		}
		if creds.Port != 6379 {
			t.Errorf("expected port 6379, got %d", creds.Port)
		}
	})

	t.Run("Error - SecretName empty", func(t *testing.T) {
		mockClient := &mockSecretsManager{}
		p := NewValkeyCredentialsProvider(mockClient, "")
		_, err := p.Get(ctx)
		if err == nil {
			t.Error("expected error due to empty secret name, got nil")
		}
	})

	t.Run("Error - GetSecretValue fails", func(t *testing.T) {
		mockErr := errors.New("access denied")
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				return nil, mockErr
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		_, err := p.Get(ctx)
		if !errors.Is(err, mockErr) {
			t.Errorf("expected error %v, got %v", mockErr, err)
		}
	})

	t.Run("Error - Empty Secrets Manager output fields", func(t *testing.T) {
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		_, err := p.Get(ctx)
		if err == nil {
			t.Error("expected error due to empty secret value payload, got nil")
		}
	})

	t.Run("Error - Invalid JSON payload", func(t *testing.T) {
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				corruptedStr := `{invalid-json`
				return &secretsmanager.GetSecretValueOutput{
					SecretString: &corruptedStr,
				}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		_, err := p.Get(ctx)
		if err == nil {
			t.Error("expected json parse error, got nil")
		}
	})

	t.Run("Error - Invalid port type (bool)", func(t *testing.T) {
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				badPortStr := `{"host": "some-host", "port": true}`
				return &secretsmanager.GetSecretValueOutput{
					SecretString: &badPortStr,
				}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		_, err := p.Get(ctx)
		if err == nil {
			t.Error("expected port type parsing error, got nil")
		}
	})

	t.Run("Error - Port string conversion fails", func(t *testing.T) {
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				badPortStr := `{"host": "some-host", "port": "not-a-number"}`
				return &secretsmanager.GetSecretValueOutput{
					SecretString: &badPortStr,
				}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		_, err := p.Get(ctx)
		if err == nil {
			t.Error("expected conversion error for non-numeric port string, got nil")
		}
	})

	t.Run("Error - Host field is empty", func(t *testing.T) {
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				emptyHostStr := `{"host": "", "port": 6379}`
				return &secretsmanager.GetSecretValueOutput{
					SecretString: &emptyHostStr,
				}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		_, err := p.Get(ctx)
		if err == nil {
			t.Error("expected error due to empty host parameter, got nil")
		}
	})

	t.Run("Concurrency - Coalescing requests (Singleflight behavior)", func(t *testing.T) {
		var callCount int32
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				atomic.AddInt32(&callCount, 1)
				time.Sleep(10 * time.Millisecond) // simulate delay
				rawJSON := `{"host": "concurrent.valkey", "port": 6379}`
				return &secretsmanager.GetSecretValueOutput{
					SecretString: &rawJSON,
				}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)

		var wg sync.WaitGroup
		numWorkers := 10
		results := make([]ValkeyCredentials, numWorkers)
		errs := make([]error, numWorkers)

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				res, err := p.Get(ctx)
				results[idx] = res
				errs[idx] = err
			}(i)
		}

		wg.Wait()

		// Verify only 1 AWS SDK request was processed
		if actualCount := atomic.LoadInt32(&callCount); actualCount != 1 {
			t.Errorf("expected Secrets Manager to be queried exactly once, was queried %d times", actualCount)
		}

		// Verify all workers resolved the same value
		for i := 0; i < numWorkers; i++ {
			if errs[i] != nil {
				t.Errorf("worker %d returned error: %v", i, errs[i])
			}
			if results[i].Host != "concurrent.valkey" {
				t.Errorf("worker %d expected host 'concurrent.valkey', got %q", i, results[i].Host)
			}
		}
	})

	t.Run("Caching - returns cached credentials on subsequent calls", func(t *testing.T) {
		var callCount int32
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				atomic.AddInt32(&callCount, 1)
				rawJSON := `{"host": "cached.valkey", "port": 6379}`
				return &secretsmanager.GetSecretValueOutput{
					SecretString: &rawJSON,
				}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		creds1, err := p.Get(ctx)
		if err != nil {
			t.Fatalf("first call failed: %v", err)
		}

		creds2, err := p.Get(ctx)
		if err != nil {
			t.Fatalf("second call failed: %v", err)
		}

		if creds1 != creds2 {
			t.Errorf("expected credentials to be identical, got %+v and %+v", creds1, creds2)
		}

		if atomic.LoadInt32(&callCount) != 1 {
			t.Errorf("expected only 1 call to Secrets Manager, got %d", callCount)
		}
	})

	t.Run("Concurrency - failure propagates to waiting workers", func(t *testing.T) {
		var callCount int32
		mockErr := errors.New("temporary aws failure")
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				atomic.AddInt32(&callCount, 1)
				time.Sleep(10 * time.Millisecond)
				return nil, mockErr
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)

		var wg sync.WaitGroup
		numWorkers := 5
		errs := make([]error, numWorkers)

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, err := p.Get(ctx)
				errs[idx] = err
			}(i)
		}

		wg.Wait()

		if atomic.LoadInt32(&callCount) != 1 {
			t.Errorf("expected 1 query to Secrets Manager, got %d", callCount)
		}

		for i := 0; i < numWorkers; i++ {
			if errs[i] == nil {
				t.Errorf("worker %d expected error, got nil", i)
			}
		}
	})

	t.Run("Success - JSON with null port falls back to 6379", func(t *testing.T) {
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				rawJSON := `{"host": "valkey-null.internal", "port": null}`
				return &secretsmanager.GetSecretValueOutput{
					SecretString: &rawJSON,
				}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		creds, err := p.Get(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if creds.Host != "valkey-null.internal" {
			t.Errorf("expected host 'valkey-null.internal', got %q", creds.Host)
		}
		if creds.Port != 6379 {
			t.Errorf("expected port 6379, got %d", creds.Port)
		}
	})

	t.Run("Success - JSON with blank port falls back to 6379", func(t *testing.T) {
		mockClient := &mockSecretsManager{
			getSecretValueFunc: func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				rawJSON := `{"host": "valkey-blank.internal", "port": "  "}`
				return &secretsmanager.GetSecretValueOutput{
					SecretString: &rawJSON,
				}, nil
			},
		}

		p := NewValkeyCredentialsProvider(mockClient, secretName)
		creds, err := p.Get(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if creds.Host != "valkey-blank.internal" {
			t.Errorf("expected host 'valkey-blank.internal', got %q", creds.Host)
		}
		if creds.Port != 6379 {
			t.Errorf("expected port 6379, got %d", creds.Port)
		}
	})
}
