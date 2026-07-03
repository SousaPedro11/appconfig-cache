package application

import (
	"context"
	"fmt"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
	"golang.org/x/sync/singleflight"
)

type CacheAsideService struct {
	source ConfigurationSource
	l1     L1Cache
	l2     L2Cache
	group  singleflight.Group
}

func NewCacheAsideService(
	source ConfigurationSource,
	l1 L1Cache,
	l2 L2Cache,
) *CacheAsideService {
	return &CacheAsideService{
		source: source,
		l1:     l1,
		l2:     l2,
	}
}

func (s *CacheAsideService) Get(ctx context.Context, application string, environment string, profile string) (domain.Configuration, error) {
	request, err := domain.NewConfigurationRequest(application, environment, profile)
	if err != nil {
		return domain.Configuration{}, err
	}

	return s.GetByRequest(ctx, request)
}

func (s *CacheAsideService) GetByRequest(ctx context.Context, request domain.ConfigurationRequest) (domain.Configuration, error) {
	key := domain.NewCacheKey(request.Application(), request.Environment())

	if cached, ok := s.l1.Get(key); ok {
		fmt.Printf("[cache hit] L1 cache para %s\n", key)
		return cached, nil
	}

	result, err, _ := s.group.Do(key.String(), func() (interface{}, error) {
		return s.resolveAndCache(ctx, key, request)
	})
	if err != nil {
		return domain.Configuration{}, err
	}

	return result.(domain.Configuration), nil
}

func (s *CacheAsideService) resolveAndCache(
	ctx context.Context,
	key domain.CacheKey,
	request domain.ConfigurationRequest,
) (domain.Configuration, error) {
	if cached, ok, err := s.l2.Get(ctx, key); err == nil && ok {
		fmt.Printf("[cache hit] L2 cache para %s\n", key)
		s.l1.Set(key, cached)
		return cached, nil
	}

	configuration, err := s.source.GetLatestConfiguration(
		ctx,
		request.Application(),
		request.Environment(),
		request.Profile(),
	)
	if err != nil {
		return domain.Configuration{}, err
	}

	fmt.Printf("[cache miss] L2 cache para %s\n", key)
	_ = s.l2.Set(ctx, key, configuration)
	s.l1.Set(key, configuration)
	return configuration, nil
}
