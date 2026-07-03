package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type ValkeyCredentials struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type SecretsManagerAPI interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

type valkeyCredentialsPayload struct {
	Host string          `json:"host"`
	Port json.RawMessage `json:"port"`
}

type ValkeyCredentialsProvider struct {
	client     SecretsManagerAPI
	secretName string

	mu         sync.Mutex
	resolved   *ValkeyCredentials
	inProgress bool
	waitCh     chan struct{}
}

func NewValkeyCredentialsProvider(client SecretsManagerAPI, secretName string) *ValkeyCredentialsProvider {
	return &ValkeyCredentialsProvider{client: client, secretName: secretName}
}

func (p *ValkeyCredentialsProvider) Get(ctx context.Context) (ValkeyCredentials, error) {
	p.mu.Lock()
	if p.resolved != nil {
		defer p.mu.Unlock()
		return *p.resolved, nil
	}

	if p.inProgress {
		wait := p.waitCh
		p.mu.Unlock()
		<-wait
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.resolved == nil {
			return ValkeyCredentials{}, errors.New("failed to resolve valkey credentials")
		}
		return *p.resolved, nil
	}

	p.inProgress = true
	p.waitCh = make(chan struct{})
	p.mu.Unlock()

	credentials, err := p.load(ctx)

	p.mu.Lock()
	if err == nil {
		p.resolved = &credentials
	}
	p.inProgress = false
	close(p.waitCh)
	p.mu.Unlock()

	if err != nil {
		return ValkeyCredentials{}, err
	}
	return credentials, nil
}

func (p *ValkeyCredentialsProvider) load(ctx context.Context) (ValkeyCredentials, error) {
	if p.secretName == "" {
		return ValkeyCredentials{}, errors.New("env CACHE_SECRET_NAME is required")
	}

	response, err := p.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId:     &p.secretName,
		VersionStage: strPtr("AWSCURRENT"),
	})
	if err != nil {
		return ValkeyCredentials{}, err
	}

	payload, err := secretPayload(response)
	if err != nil {
		return ValkeyCredentials{}, err
	}

	credentials, err := parseCredentials(payload)
	if err != nil {
		return ValkeyCredentials{}, fmt.Errorf("invalid json inside valkey secret: %w", err)
	}

	if credentials.Host == "" {
		return ValkeyCredentials{}, errors.New("host field is required in valkey secret")
	}
	if credentials.Port == 0 {
		credentials.Port = 6379
	}

	return credentials, nil
}

func parseCredentials(payload string) (ValkeyCredentials, error) {
	var parsed valkeyCredentialsPayload
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return ValkeyCredentials{}, err
	}

	port, err := parsePort(parsed.Port)
	if err != nil {
		return ValkeyCredentials{}, err
	}

	return ValkeyCredentials{
		Host: parsed.Host,
		Port: port,
	}, nil
}

func parsePort(raw json.RawMessage) (int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}

	var asInt int
	if err := json.Unmarshal(raw, &asInt); err == nil {
		return asInt, nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		asString = strings.TrimSpace(asString)
		if asString == "" {
			return 0, nil
		}

		value, convErr := strconv.Atoi(asString)
		if convErr != nil {
			return 0, fmt.Errorf("invalid port field: %s", asString)
		}

		return value, nil
	}

	return 0, errors.New("port field must be a number or a numeric string")
}

func secretPayload(response *secretsmanager.GetSecretValueOutput) (string, error) {
	if response.SecretString != nil {
		return *response.SecretString, nil
	}

	if response.SecretBinary != nil {
		return string(response.SecretBinary), nil
	}

	return "", errors.New("empty secret payload for CACHE_SECRET_NAME")
}

func strPtr(value string) *string {
	return &value
}
