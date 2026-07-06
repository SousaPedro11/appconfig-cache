package configpayload

import "testing"

func TestPayloadMergeMissing(t *testing.T) {
	t.Run("Merge with partially filled base", func(t *testing.T) {
		base := Payload{Application: "app"}
		base.MergeMissing(Payload{Application: "other", Environment: "prd", Profile: "default"})

		if base.Application != "app" {
			t.Fatalf("application foi sobrescrita: %q", base.Application)
		}
		if base.Environment != "prd" || base.Profile != "default" {
			t.Fatalf("merge inesperado: %+v", base)
		}
	})

	t.Run("Merge with empty base fields", func(t *testing.T) {
		base := Payload{}
		base.MergeMissing(Payload{Application: "app", Environment: "prd", Profile: "default"})

		if base.Application != "app" || base.Environment != "prd" || base.Profile != "default" {
			t.Fatalf("merge falhou em preencher campos vazios: %+v", base)
		}
	})
}

func TestPayloadValidate(t *testing.T) {
	t.Run("Valid Payload", func(t *testing.T) {
		if err := (Payload{Application: "app", Environment: "prd", Profile: "default"}).Validate(); err != nil {
			t.Fatalf("validate falhou em payload válido: %v", err)
		}
	})

	t.Run("Missing Fields Validation Failure", func(t *testing.T) {
		if err := (Payload{Application: "app", Environment: "prd"}).Validate(); err == nil {
			t.Fatalf("validate deveria falhar para payload inválido")
		}
	})
}

func TestParseJSON(t *testing.T) {
	t.Run("Valid JSON", func(t *testing.T) {
		body := []byte(`{"application":"app","environment":"dev","profile":"default"}`)
		payload, err := ParseJSON(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if payload.Application != "app" || payload.Environment != "dev" || payload.Profile != "default" {
			t.Errorf("unexpected payload parsed: %+v", payload)
		}
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		body := []byte(`{invalid-json}`)
		_, err := ParseJSON(body)
		if err == nil {
			t.Error("expected error for invalid JSON, got nil")
		}
	})
}
