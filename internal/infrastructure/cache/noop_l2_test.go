package cache

import (
	"context"
	"testing"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

func TestNoOpL2Cache(t *testing.T) {
	c := NewNoOpL2Cache()
	ctx := context.Background()
	key := domain.NewCacheKey(domain.ApplicationID("test-app"), domain.EnvironmentID("prd"))
	config, err := domain.NewConfiguration(`{"feature": true}`)
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	t.Run("Get always returns empty and false", func(t *testing.T) {
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
	})

	t.Run("Set always returns nil", func(t *testing.T) {
		gotErr := c.Set(ctx, key, config)
		if gotErr != nil {
			t.Errorf("unexpected error: %v", gotErr)
		}
	})
}
