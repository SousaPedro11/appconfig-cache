package domain

import (
	"errors"
	"strings"
)

var ErrEmptyConfiguration = errors.New("configuration content cannot be empty")

type Configuration struct {
	content string
}

func NewConfiguration(content string) (Configuration, error) {
	if strings.TrimSpace(content) == "" {
		return Configuration{}, ErrEmptyConfiguration
	}

	return Configuration{content: content}, nil
}

func (c Configuration) Content() string {
	return c.content
}
