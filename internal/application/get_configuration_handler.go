package application

import (
	"context"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

type GetConfigurationHandler struct {
	service *CacheAsideService
}

func NewGetConfigurationHandler(service *CacheAsideService) *GetConfigurationHandler {
	return &GetConfigurationHandler{service: service}
}
func (h *GetConfigurationHandler) Handle(ctx context.Context, command GetConfigurationCommand) (string, error) {
	request, err := domain.NewConfigurationRequest(command.Application, command.Environment, command.Profile)
	if err != nil {
		return "", err
	}

	config, err := h.service.GetByRequest(ctx, request)
	if err != nil {
		return "", err
	}

	return config.Content(), nil
}
