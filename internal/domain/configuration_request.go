package domain

import (
	"errors"
	"strings"
)

var ErrInvalidConfigurationRequest = errors.New("application, environment and profile are required")

type ApplicationID string
type EnvironmentID string
type ProfileID string

type ConfigurationRequest struct {
	application ApplicationID
	environment EnvironmentID
	profile     ProfileID
}

func NewConfigurationRequest(application string, environment string, profile string) (ConfigurationRequest, error) {
	app := ApplicationID(strings.TrimSpace(application))
	env := EnvironmentID(strings.TrimSpace(environment))
	prof := ProfileID(strings.TrimSpace(profile))

	if app == "" || env == "" || prof == "" {
		return ConfigurationRequest{}, ErrInvalidConfigurationRequest
	}

	return ConfigurationRequest{
		application: app,
		environment: env,
		profile:     prof,
	}, nil
}

func (r ConfigurationRequest) Application() ApplicationID {
	return r.application
}

func (r ConfigurationRequest) Environment() EnvironmentID {
	return r.environment
}

func (r ConfigurationRequest) Profile() ProfileID {
	return r.profile
}
