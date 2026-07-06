package configpayload

import (
	"encoding/json"
	"errors"
)

var ErrMissingFields = errors.New("application, environment and profile are required")

type Payload struct {
	Application string `json:"application"`
	Environment string `json:"environment"`
	Profile     string `json:"profile"`
}

func (p *Payload) MergeMissing(other Payload) {
	if p.Application == "" {
		p.Application = other.Application
	}
	if p.Environment == "" {
		p.Environment = other.Environment
	}
	if p.Profile == "" {
		p.Profile = other.Profile
	}
}

func (p Payload) Validate() error {
	if p.Application == "" || p.Environment == "" || p.Profile == "" {
		return ErrMissingFields
	}

	return nil
}

func ParseJSON(body []byte) (Payload, error) {
	var payload Payload
	if err := json.Unmarshal(body, &payload); err != nil {
		return Payload{}, err
	}

	return payload, nil
}
