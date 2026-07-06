package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/sousapedro11/appconfig-cache/internal/application"
	"github.com/sousapedro11/appconfig-cache/internal/bootstrap"
	"github.com/sousapedro11/appconfig-cache/internal/domain"
	"github.com/sousapedro11/appconfig-cache/internal/transport/auth"
	"github.com/sousapedro11/appconfig-cache/internal/transport/configpayload"
)

type ErrorBody struct {
	Message string `json:"message"`
}

const (
	contentTypeHeader = "Content-Type"
	contentTypeJSON   = "application/json"
)

var (
	once             = &sync.Once{}
	getConfiguration *application.GetConfigurationHandler
	apiKeyValidator  auth.APIKeyValidator
	initErr          error
)

var buildRuntime = bootstrap.BuildRuntime

func ensureRuntime(ctx context.Context) error {
	once.Do(func() {
		runtime, err := buildRuntime(ctx)
		if err != nil {
			initErr = err
			return
		}

		getConfiguration = runtime.GetConfiguration
		apiKeyValidator = auth.NewAPIKeyValidator(os.Getenv("X_API_KEY"))
	})

	return initErr
}

func resetRuntimeState() {
	once = &sync.Once{}
	getConfiguration = nil
	apiKeyValidator = auth.APIKeyValidator{}
	initErr = nil
}

func authorizeRequest(request events.APIGatewayProxyRequest) error {
	headerValue := auth.FindHeaderCaseInsensitive(request.Headers, auth.HeaderName)
	queryValue := request.QueryStringParameters[auth.QueryKey]
	return apiKeyValidator.Validate(headerValue, queryValue)
}

func fillPayloadFromPathParams(payload *configpayload.Payload, pathParams map[string]string) {
	payload.MergeMissing(configpayload.Payload{
		Application: pathParams["application"],
		Environment: pathParams["environment"],
		Profile:     pathParams["profile"],
	})
}

func mergePayloadFromBody(payload *configpayload.Payload, body string) {
	if body == "" {
		return
	}

	bodyPayload, err := configpayload.ParseJSON([]byte(body))
	if err != nil {
		return
	}
	payload.MergeMissing(bodyPayload)
}

func buildPayload(request events.APIGatewayProxyRequest) (configpayload.Payload, error) {
	payload := configpayload.Payload{
		Application: request.QueryStringParameters["application"],
		Environment: request.QueryStringParameters["environment"],
		Profile:     request.QueryStringParameters["profile"],
	}
	fillPayloadFromPathParams(&payload, request.PathParameters)
	mergePayloadFromBody(&payload, request.Body)

	if err := payload.Validate(); err != nil {
		return configpayload.Payload{}, err
	}

	return payload, nil
}

func badRequest(err error) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(ErrorBody{Message: err.Error()})
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusBadRequest,
		Headers: map[string]string{
			contentTypeHeader: contentTypeJSON,
		},
		Body: string(body),
	}
}

var successBodyFn = successBody

func successBody(configuration string) ([]byte, error) {
	if json.Valid([]byte(configuration)) {
		return []byte(configuration), nil
	}

	return json.Marshal(map[string]string{"configuration": configuration})
}

func unauthorized(err error) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(ErrorBody{Message: err.Error()})
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusUnauthorized,
		Headers: map[string]string{
			contentTypeHeader: contentTypeJSON,
		},
		Body: string(body),
	}
}

func internalServerError(err error) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(ErrorBody{Message: err.Error()})
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusInternalServerError,
		Headers: map[string]string{
			contentTypeHeader: contentTypeJSON,
		},
		Body: string(body),
	}
}

func isBadRequestError(err error) bool {
	return errors.Is(err, configpayload.ErrMissingFields) || errors.Is(err, domain.ErrInvalidConfigurationRequest)
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if err := ensureRuntime(ctx); err != nil {
		return internalServerError(err), nil
	}

	if err := authorizeRequest(request); err != nil {
		return unauthorized(err), nil
	}

	payload, err := buildPayload(request)
	if err != nil {
		return badRequest(err), nil
	}

	configuration, err := getConfiguration.Handle(ctx, application.GetConfigurationCommand{
		Application: payload.Application,
		Environment: payload.Environment,
		Profile:     payload.Profile,
	})
	if err != nil {
		if isBadRequestError(err) {
			return badRequest(err), nil
		}
		return internalServerError(err), nil
	}

	body, err := successBodyFn(configuration)
	if err != nil {
		return internalServerError(err), nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			contentTypeHeader: contentTypeJSON,
		},
		Body: string(body),
	}, nil
}

var lambdaStart = lambda.Start

func main() {
	lambdaStart(handler)
}
