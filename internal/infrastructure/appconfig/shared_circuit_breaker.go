package appconfig

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

var marshalMap = attributevalue.MarshalMap

type DynamoDBAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

type SharedCircuitBreaker struct {
	client           DynamoDBAPI
	tableName        string
	failureThreshold int
	timeout          time.Duration
	localFallback    *CircuitBreaker // Fallback if DynamoDB is unavailable
	mu               sync.Mutex
	lastDynamoError  time.Time
	dynamoRetryAfter time.Duration
}

type circuitState struct {
	Application     string `dynamodbav:"PK"`
	Environment     string `dynamodbav:"SK"`
	State           string `dynamodbav:"state"`
	FailureCount    int    `dynamodbav:"failureCount"`
	LastFailureTime int64  `dynamodbav:"lastFailureTime"`
	TTL             int64  `dynamodbav:"ttl"`
}

func NewSharedCircuitBreaker(
	cfg aws.Config,
	tableName string,
	failureThreshold int,
	timeout time.Duration,
) *SharedCircuitBreaker {
	return &SharedCircuitBreaker{
		client:           dynamodb.NewFromConfig(cfg),
		tableName:        tableName,
		failureThreshold: failureThreshold,
		timeout:          timeout,
		localFallback:    NewCircuitBreaker(failureThreshold, timeout),
		dynamoRetryAfter: 5 * time.Second,
	}
}

// Call executes fn with shared circuit breaker state from DynamoDB
// If DynamoDB fails, falls back to local circuit breaker
func (scb *SharedCircuitBreaker) Call(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID, fn func() error) error {

	if scb.shouldUseFallback() {
		return scb.localFallback.Call(fn)
	}

	state, err := scb.getState(ctx, application, environment)
	if err != nil {

		scb.recordDynamoError()
		return scb.localFallback.Call(fn)
	}

	if state.State == string(StateOpen) {
		if time.Since(time.UnixMilli(state.LastFailureTime)) <= scb.timeout {
			return ErrCircuitOpen
		}
		state.State = string(StateHalfOpen)
		state.FailureCount = 0
	}

	fnErr := fn()

	if fnErr != nil {
		state.FailureCount++
		state.LastFailureTime = time.Now().UnixMilli()

		if state.FailureCount >= scb.failureThreshold || state.State == string(StateHalfOpen) {
			state.State = string(StateOpen)
		}

		_ = scb.setState(ctx, state)

		return fnErr
	}

	state.State = string(StateClosed)
	state.FailureCount = 0
	_ = scb.setState(ctx, state)

	return nil
}

func (scb *SharedCircuitBreaker) getState(ctx context.Context, application domain.ApplicationID, environment domain.EnvironmentID) (*circuitState, error) {
	appStr := string(application)
	envStr := string(environment)

	result, err := scb.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(scb.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: appStr},
			"SK": &types.AttributeValueMemberS{Value: envStr},
		},
	})
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return &circuitState{
			Application:     appStr,
			Environment:     envStr,
			State:           string(StateClosed),
			FailureCount:    0,
			LastFailureTime: 0,
			TTL:             time.Now().Add(1 * time.Hour).Unix(),
		}, nil
	}

	state := &circuitState{}
	err = attributevalue.UnmarshalMap(result.Item, state)
	if err != nil {
		return nil, err
	}

	return state, nil
}

func (scb *SharedCircuitBreaker) setState(ctx context.Context, state *circuitState) error {
	state.TTL = time.Now().Add(1 * time.Hour).Unix()

	item, err := marshalMap(state)
	if err != nil {
		return err
	}

	_, err = scb.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(scb.tableName),
		Item:      item,
	})

	if err == nil {
		scb.recordDynamoSuccess()
	}

	return err
}

func (scb *SharedCircuitBreaker) shouldUseFallback() bool {
	scb.mu.Lock()
	defer scb.mu.Unlock()

	if scb.lastDynamoError.IsZero() {
		return false
	}

	return time.Since(scb.lastDynamoError) < scb.dynamoRetryAfter
}

func (scb *SharedCircuitBreaker) recordDynamoError() {
	scb.mu.Lock()
	defer scb.mu.Unlock()
	scb.lastDynamoError = time.Now()
}

func (scb *SharedCircuitBreaker) recordDynamoSuccess() {
	scb.mu.Lock()
	defer scb.mu.Unlock()
	scb.lastDynamoError = time.Time{}
}

func (scb *SharedCircuitBreaker) GetState(ctx context.Context, application string, environment string) (string, error) {
	state, err := scb.getState(ctx, domain.ApplicationID(application), domain.EnvironmentID(environment))
	if err != nil {
		return "", err
	}
	return state.State, nil
}

func (scb *SharedCircuitBreaker) ListStates(ctx context.Context, application string) ([]circuitState, error) {
	result, err := scb.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(scb.tableName),
		KeyConditionExpression: aws.String("PK = :app"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":app": &types.AttributeValueMemberS{Value: application},
		},
	})
	if err != nil {
		return nil, err
	}

	var states []circuitState
	err = attributevalue.UnmarshalListOfMaps(result.Items, &states)
	if err != nil {
		return nil, err
	}

	return states, nil
}
