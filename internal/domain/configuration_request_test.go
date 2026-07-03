package domain

import (
	"errors"
	"testing"
)

func TestNewConfigurationRequest(t *testing.T) {
	tests := []struct {
		name        string
		application string
		environment string
		profile     string
		wantErr     error
	}{
		{
			name:        "valid request",
			application: "my-app",
			environment: "prod",
			profile:     "default",
			wantErr:     nil,
		},
		{
			name:        "missing application",
			application: "",
			environment: "prod",
			profile:     "default",
			wantErr:     ErrInvalidConfigurationRequest,
		},
		{
			name:        "missing environment",
			application: "my-app",
			environment: "",
			profile:     "default",
			wantErr:     ErrInvalidConfigurationRequest,
		},
		{
			name:        "missing profile",
			application: "my-app",
			environment: "prod",
			profile:     "",
			wantErr:     ErrInvalidConfigurationRequest,
		},
		{
			name:        "spaces only",
			application: "  ",
			environment: "prod",
			profile:     "default",
			wantErr:     ErrInvalidConfigurationRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := NewConfigurationRequest(tt.application, tt.environment, tt.profile)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(req.Application()) != tt.application {
				t.Errorf("expected application %q, got %q", tt.application, req.Application())
			}
			if string(req.Environment()) != tt.environment {
				t.Errorf("expected environment %q, got %q", tt.environment, req.Environment())
			}
			if string(req.Profile()) != tt.profile {
				t.Errorf("expected profile %q, got %q", tt.profile, req.Profile())
			}
		})
	}
}
