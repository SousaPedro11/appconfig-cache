package application

import (
	"context"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

type ConfigurationSource interface {
	GetLatestConfiguration(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error)
}

type L1Cache interface {
	Get(key domain.CacheKey) (domain.Configuration, bool)
	Set(key domain.CacheKey, value domain.Configuration)
}

type L2Cache interface {
	Get(ctx context.Context, key domain.CacheKey) (domain.Configuration, bool, error)
	Set(ctx context.Context, key domain.CacheKey, value domain.Configuration) error
}
