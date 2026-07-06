package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

func TestValkeyL2Cache(t *testing.T) {
	ctx := context.Background()
	key := domain.NewCacheKey(domain.ApplicationID("test-app"), domain.EnvironmentID("prd"))
	config, err := domain.NewConfiguration(`{"feature": true}`)
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	t.Run("Get - Cache Hit", func(t *testing.T) {
		db, mock := redismock.NewClientMock()
		c := NewValkeyL2Cache(db, 5*time.Minute)

		mock.ExpectGet(key.String()).SetVal(`{"feature": true}`)

		gotConfig, ok, gotErr := c.Get(ctx, key)
		if gotErr != nil {
			t.Errorf("unexpected error: %v", gotErr)
		}
		if !ok {
			t.Error("expected ok to be true")
		}
		if gotConfig.Content() != config.Content() {
			t.Errorf("expected config content %q, got %q", config.Content(), gotConfig.Content())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet mock expectations: %v", err)
		}
	})

	t.Run("Get - Cache Miss", func(t *testing.T) {
		db, mock := redismock.NewClientMock()
		c := NewValkeyL2Cache(db, 5*time.Minute)

		mock.ExpectGet(key.String()).RedisNil()

		gotConfig, ok, gotErr := c.Get(ctx, key)
		if gotErr != nil {
			t.Errorf("unexpected error: %v", gotErr)
		}
		if ok {
			t.Error("expected ok to be false")
		}
		if gotConfig.Content() != "" {
			t.Errorf("expected empty config content, got %q", gotConfig.Content())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet mock expectations: %v", err)
		}
	})

	t.Run("Get - Valkey Error", func(t *testing.T) {
		db, mock := redismock.NewClientMock()
		c := NewValkeyL2Cache(db, 5*time.Minute)

		expectedErr := errors.New("valkey connection timeout")
		mock.ExpectGet(key.String()).SetErr(expectedErr)

		_, ok, gotErr := c.Get(ctx, key)
		if !errors.Is(gotErr, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, gotErr)
		}
		if ok {
			t.Error("expected ok to be false")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet mock expectations: %v", err)
		}
	})

	t.Run("Get - Invalid Configuration JSON stored in Valkey", func(t *testing.T) {
		db, mock := redismock.NewClientMock()
		c := NewValkeyL2Cache(db, 5*time.Minute)

		mock.ExpectGet(key.String()).SetVal("")

		_, ok, gotErr := c.Get(ctx, key)
		if gotErr == nil {
			t.Error("expected error due to empty JSON string, got nil")
		}
		if ok {
			t.Error("expected ok to be false")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet mock expectations: %v", err)
		}
	})

	t.Run("Set - Success", func(t *testing.T) {
		db, mock := redismock.NewClientMock()
		ttl := 5 * time.Minute
		c := NewValkeyL2Cache(db, ttl)

		mock.ExpectSet(key.String(), config.Content(), ttl).SetVal("OK")

		gotErr := c.Set(ctx, key, config)
		if gotErr != nil {
			t.Errorf("unexpected error: %v", gotErr)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet mock expectations: %v", err)
		}
	})

	t.Run("Set - Valkey Error", func(t *testing.T) {
		db, mock := redismock.NewClientMock()
		ttl := 5 * time.Minute
		c := NewValkeyL2Cache(db, ttl)

		expectedErr := errors.New("valkey readonly mode")
		mock.ExpectSet(key.String(), config.Content(), ttl).SetErr(expectedErr)

		gotErr := c.Set(ctx, key, config)
		if !errors.Is(gotErr, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, gotErr)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet mock expectations: %v", err)
		}
	})
}
