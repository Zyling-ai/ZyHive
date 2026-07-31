package config

import (
	"path/filepath"
	"testing"
)

func TestGatewayConfigValidate(t *testing.T) {
	for _, gateway := range []GatewayConfig{
		{},
		{Port: 8080, Bind: "localhost"},
		{Port: 8080, Bind: "lan"},
		{Port: 8080, Bind: "all"},
		{Port: 8080, Bind: "10.0.0.5"},
		{Port: 8080, Bind: "::1"},
	} {
		if err := gateway.Validate(); err != nil {
			t.Fatalf("%+v: %v", gateway, err)
		}
	}
}

func TestGatewayConfigValidateRejectsInvalidValues(t *testing.T) {
	for _, gateway := range []GatewayConfig{
		{Port: -1, Bind: "localhost"},
		{Port: 65536, Bind: "localhost"},
		{Port: 8080, Bind: "internet"},
	} {
		if err := gateway.Validate(); err == nil {
			t.Fatalf("expected invalid gateway: %+v", gateway)
		}
	}
}

func TestDefaultUsesUniqueSecureToken(t *testing.T) {
	first := Default().Auth.Token
	second := Default().Auth.Token
	if len(first) != 32 {
		t.Fatalf("token length = %d, want 32 hex chars", len(first))
	}
	if first == "changeme" || first == second {
		t.Fatal("default tokens must be random and unique")
	}
}

func TestSaveRejectsInvalidGateway(t *testing.T) {
	cfg := Default()
	cfg.Gateway.Bind = "internet"
	if err := Save(filepath.Join(t.TempDir(), "config.json"), cfg); err == nil {
		t.Fatal("expected invalid gateway error")
	}
}
