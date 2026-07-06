package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

type ValkeyL2Cache struct {
	client redis.Cmdable
	ttl    time.Duration
}

func NewValkeyL2Cache(client redis.Cmdable, ttl time.Duration) *ValkeyL2Cache {
	return &ValkeyL2Cache{client: client, ttl: ttl}
}

func (c *ValkeyL2Cache) Get(ctx context.Context, key domain.CacheKey) (domain.Configuration, bool, error) {
	value, err := c.client.Get(ctx, key.String()).Result()
	if err == redis.Nil {
		return domain.Configuration{}, false, nil
	}
	if err != nil {
		return domain.Configuration{}, false, err
	}

	config, err := domain.NewConfiguration(value)
	if err != nil {
		return domain.Configuration{}, false, err
	}

	return config, true, nil
}

func (c *ValkeyL2Cache) Set(ctx context.Context, key domain.CacheKey, value domain.Configuration) error {
	return c.client.Set(ctx, key.String(), value.Content(), c.ttl).Err()
}
