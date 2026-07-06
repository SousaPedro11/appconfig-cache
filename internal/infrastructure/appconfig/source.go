package appconfig

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/appconfigdata"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

type AppConfigDataAPI interface {
	StartConfigurationSession(ctx context.Context, params *appconfigdata.StartConfigurationSessionInput, optFns ...func(*appconfigdata.Options)) (*appconfigdata.StartConfigurationSessionOutput, error)
	GetLatestConfiguration(ctx context.Context, params *appconfigdata.GetLatestConfigurationInput, optFns ...func(*appconfigdata.Options)) (*appconfigdata.GetLatestConfigurationOutput, error)
}

type Source struct {
	cfg                  aws.Config
	once                 sync.Once
	client               AppConfigDataAPI
	circuitBreaker       *CircuitBreaker
	sharedCircuitBreaker *SharedCircuitBreaker
	requestTimeout       time.Duration
}

const defaultRequestTimeout = 5 * time.Second

func NewSource(cfg aws.Config) *Source {
	return &Source{
		cfg:            cfg,
		circuitBreaker: NewCircuitBreaker(5, 30*time.Second),
		requestTimeout: defaultRequestTimeout,
	}
}

func (s *Source) WithRequestTimeout(timeout time.Duration) *Source {
	if timeout > 0 {
		s.requestTimeout = timeout
	}

	return s
}

func (s *Source) WithSharedCircuitBreaker(scb *SharedCircuitBreaker) *Source {
	s.sharedCircuitBreaker = scb
	return s
}

func (s *Source) GetLatestConfiguration(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error) {
	if application == "" || environment == "" || profile == "" {
		return domain.Configuration{}, errors.New("application, environment and profile are required")
	}

	if s.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.requestTimeout)
		defer cancel()
	}

	// Use shared circuit breaker if available, otherwise use local
	if s.sharedCircuitBreaker != nil {
		return s.getConfigWithSharedCircuitBreaker(ctx, application, environment, profile)
	}

	var result string
	err := s.circuitBreaker.Call(func() error {
		return s.fetchConfiguration(ctx, application, environment, profile, &result)
	})
	if err != nil {
		return domain.Configuration{}, err
	}

	return domain.NewConfiguration(result)
}

func (s *Source) getConfigWithSharedCircuitBreaker(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID) (domain.Configuration, error) {
	var result string
	err := s.sharedCircuitBreaker.Call(ctx, application, environment, func() error {
		return s.fetchConfiguration(ctx, application, environment, profile, &result)
	})
	if err != nil {
		return domain.Configuration{}, err
	}
	return domain.NewConfiguration(result)
}

func (s *Source) fetchConfiguration(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, profile domain.ProfileID, result *string) error {
	client := s.getClient()

	appStr := string(application)
	envStr := string(environment)
	profStr := string(profile)

	sessionOutput, err := client.StartConfigurationSession(ctx, &appconfigdata.StartConfigurationSessionInput{
		ApplicationIdentifier:          &appStr,
		EnvironmentIdentifier:          &envStr,
		ConfigurationProfileIdentifier: &profStr,
	})
	if err != nil {
		return err
	}

	if sessionOutput.InitialConfigurationToken == nil || *sessionOutput.InitialConfigurationToken == "" {
		return errors.New("empty AppConfig initial configuration token")
	}

	configOutput, err := client.GetLatestConfiguration(ctx, &appconfigdata.GetLatestConfigurationInput{
		ConfigurationToken: sessionOutput.InitialConfigurationToken,
	})
	if err != nil {
		return err
	}

	if len(configOutput.Configuration) == 0 {
		return fmt.Errorf("configuration not found for %s/%s/%s", application, environment, profile)
	}

	*result = string(configOutput.Configuration)
	return nil
}

func (s *Source) getClient() AppConfigDataAPI {
	s.once.Do(func() {
		if s.client == nil {
			s.client = appconfigdata.NewFromConfig(s.cfg)
		}
	})

	return s.client
}
