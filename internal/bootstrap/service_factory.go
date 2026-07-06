package bootstrap

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/redis/go-redis/v9"

	"github.com/sousapedro11/appconfig-cache/internal/application"
	"github.com/sousapedro11/appconfig-cache/internal/infrastructure/appconfig"
	"github.com/sousapedro11/appconfig-cache/internal/infrastructure/cache"
	"github.com/sousapedro11/appconfig-cache/internal/infrastructure/secrets"
)

var loadAWSConfig = func(ctx context.Context, region string) (aws.Config, error) {
	return awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
}

var pingRedis = func(ctx context.Context, client *redis.Client) error {
	return client.Ping(ctx).Err()
}

const (
	defaultL1TTLSeconds            = 120
	defaultL2TTLSeconds            = 86400
	defaultAppConfigTimeoutSeconds = 5
)

type Runtime struct {
	Service          *application.CacheAsideService
	GetConfiguration *application.GetConfigurationHandler
	Close            func() error
}

func BuildRuntime(ctx context.Context) (Runtime, error) {
	region := envOr("AWS_REGION", "us-east-1")

	cfg, err := loadAWSConfig(ctx, region)
	if err != nil {
		return Runtime{}, err
	}

	source := appconfig.NewSource(cfg)
	source.WithRequestTimeout(time.Duration(intFromEnv("APPCONFIG_TIMEOUT_SECONDS", defaultAppConfigTimeoutSeconds)) * time.Second)

	if tableName := os.Getenv("CIRCUIT_BREAKER_TABLE_NAME"); tableName != "" {
		scb := appconfig.NewSharedCircuitBreaker(cfg, tableName, 5, 30*time.Second)
		source.WithSharedCircuitBreaker(scb)
		fmt.Printf("[info] circuit breaker compartilhado ativo: %s\n", tableName)
	}

	l1TTL := time.Duration(intFromEnv("L1_TTL_SECONDS", defaultL1TTLSeconds)) * time.Second
	l2TTL := time.Duration(intFromEnv("L2_TTL_SECONDS", defaultL2TTLSeconds)) * time.Second

	l1 := cache.NewInMemoryL1Cache(l1TTL)
	l2, closeFn := buildL2Cache(ctx, cfg, l2TTL)

	service := application.NewCacheAsideService(
		source,
		l1,
		l2,
	)
	getConfiguration := application.NewGetConfigurationHandler(service)

	return Runtime{Service: service, GetConfiguration: getConfiguration, Close: closeFn}, nil
}

func buildL2Cache(ctx context.Context, cfg aws.Config, ttl time.Duration) (application.L2Cache, func() error) {
	host := os.Getenv("VALKEY_HOST")
	if host != "" {
		port := intFromEnv("VALKEY_PORT", 6379)
		return buildValkeyClient(ctx, host, port, ttl)
	}

	if os.Getenv("VALKEY_PORT") != "" {
		fmt.Printf("[warn] VALKEY_PORT foi definida sem VALKEY_HOST; usando somente L1\n")
		return cache.NewNoOpL2Cache(), noopClose
	}

	secretName := os.Getenv("CACHE_SECRET_NAME")
	if secretName == "" {
		return cache.NewNoOpL2Cache(), noopClose
	}

	provider := secrets.NewValkeyCredentialsProvider(secretsmanager.NewFromConfig(cfg), secretName)
	creds, err := provider.Get(ctx)
	if err != nil {
		fmt.Printf("[warn] falha ao carregar credenciais valkey: %v\n", err)
		return cache.NewNoOpL2Cache(), noopClose
	}

	return buildValkeyClient(ctx, creds.Host, creds.Port, ttl)
}

func buildValkeyClient(ctx context.Context, host string, port int, ttl time.Duration) (application.L2Cache, func() error) {
	redisClient := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", host, port),
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	})

	if err := pingRedis(ctx, redisClient); err != nil {
		fmt.Printf("[warn] valkey indisponível, usando somente L1: %v\n", err)
		_ = redisClient.Close()
		return cache.NewNoOpL2Cache(), noopClose
	}

	return cache.NewValkeyL2Cache(redisClient, ttl), redisClient.Close
}

func envOr(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func intFromEnv(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func noopClose() error {
	return nil
}
