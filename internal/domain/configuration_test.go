package domain

import (
	"errors"
	"testing"
)

func TestNewConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr error
	}{
		{
			name:    "valid configuration",
			content: `{"enabled": true}`,
			wantErr: nil,
		},
		{
			name:    "empty configuration",
			content: "",
			wantErr: ErrEmptyConfiguration,
		},
		{
			name:    "spaces only configuration",
			content: "   ",
			wantErr: ErrEmptyConfiguration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewConfiguration(tt.content)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Content() != tt.content {
				t.Errorf("expected content %q, got %q", tt.content, got.Content())
			}
		})
	}
}
