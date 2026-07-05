package auth

import (
	"errors"
	"testing"
)

func TestAPIKeyValidator_Validate(t *testing.T) {
	tests := []struct {
		name          string
		expected      string
		headerVal     string
		queryVal      string
		expectedError error
	}{
		{
			name:          "Bypass when expected is empty",
			expected:      "",
			headerVal:     "any-token",
			queryVal:      "",
			expectedError: nil,
		},
		{
			name:          "Valid token in header",
			expected:      "secret-token",
			headerVal:     "secret-token",
			queryVal:      "",
			expectedError: nil,
		},
		{
			name:          "Valid token in query",
			expected:      "secret-token",
			headerVal:     "",
			queryVal:      "secret-token",
			expectedError: nil,
		},
		{
			name:          "Invalid token in header",
			expected:      "secret-token",
			headerVal:     "wrong-token",
			queryVal:      "",
			expectedError: ErrUnauthorized,
		},
		{
			name:          "Invalid token in query",
			expected:      "secret-token",
			headerVal:     "",
			queryVal:      "wrong-token",
			expectedError: ErrUnauthorized,
		},
		{
			name:          "Missing token",
			expected:      "secret-token",
			headerVal:     "",
			queryVal:      "",
			expectedError: ErrUnauthorized,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := NewAPIKeyValidator(tc.expected)
			err := v.Validate(tc.headerVal, tc.queryVal)

			if tc.expectedError != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, tc.expectedError) {
					t.Errorf("expected error %v, got %v", tc.expectedError, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestFindHeaderCaseInsensitive(t *testing.T) {
	headers := map[string]string{
		"X-Api-Token":     "token-value",
		"Content-Type":     "application/json",
		"x-another-header": "another-value",
	}

	tests := []struct {
		name          string
		headerName    string
		expectedValue string
	}{
		{
			name:          "Exact match",
			headerName:    "X-Api-Token",
			expectedValue: "token-value",
		},
		{
			name:          "Case insensitive lowercase",
			headerName:    "x-api-token",
			expectedValue: "token-value",
		},
		{
			name:          "Case insensitive uppercase",
			headerName:    "X-ANOTHER-HEADER",
			expectedValue: "another-value",
		},
		{
			name:          "Not found",
			headerName:    "Authorization",
			expectedValue: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FindHeaderCaseInsensitive(headers, tc.headerName)
			if got != tc.expectedValue {
				t.Errorf("expected %q, got %q", tc.expectedValue, got)
			}
		})
	}
}
