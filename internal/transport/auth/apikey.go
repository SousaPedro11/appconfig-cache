package auth

import (
	"crypto/subtle"
	"errors"
	"strings"
)

const (
	HeaderName = "x-api-token"
	QueryKey   = "x-api-token"
)

var ErrUnauthorized = errors.New("invalid api key")

type APIKeyValidator struct {
	expected string
}

func NewAPIKeyValidator(expected string) APIKeyValidator {
	return APIKeyValidator{expected: expected}
}

func (v APIKeyValidator) Validate(headerValue string, queryValue string) error {
	if strings.TrimSpace(v.expected) == "" {
		return nil
	}

	provided := strings.TrimSpace(headerValue)
	if provided == "" {
		provided = strings.TrimSpace(queryValue)
	}

	if subtle.ConstantTimeCompare([]byte(provided), []byte(v.expected)) != 1 {
		return ErrUnauthorized
	}

	return nil
}

func FindHeaderCaseInsensitive(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}

	return ""
}
