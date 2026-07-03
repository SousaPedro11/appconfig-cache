package cache

import (
	"context"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

type NoOpL2Cache struct{}

func NewNoOpL2Cache() *NoOpL2Cache {
	return &NoOpL2Cache{}
}

func (c *NoOpL2Cache) Get(_ context.Context, _ domain.CacheKey) (domain.Configuration, bool, error) {
	return domain.Configuration{}, false, nil
}

func (c *NoOpL2Cache) Set(_ context.Context, _ domain.CacheKey, _ domain.Configuration) error {
	return nil
}
