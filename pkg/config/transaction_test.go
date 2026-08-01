package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestTransactionDoesNotPublishFailedWrite(t *testing.T) {
	cfg := Default()
	originalPort := cfg.Gateway.Port
	path := filepath.Join(t.TempDir(), "config-target")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	err := Transaction(path, cfg, func(candidate *Config) error {
		candidate.Gateway.Port = 9090
		return nil
	})
	if err == nil {
		t.Fatal("expected replacing a directory to fail")
	}
	if cfg.Gateway.Port != originalPort {
		t.Fatalf("failed transaction published port %d", cfg.Gateway.Port)
	}
}

func TestTransactionSerializesConcurrentUpdates(t *testing.T) {
	cfg := Default()
	path := filepath.Join(t.TempDir(), "aipanel.json")
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	const count = 40
	var wait sync.WaitGroup
	for i := 0; i < count; i++ {
		i := i
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := Transaction(path, cfg, func(candidate *Config) error {
				candidate.Providers = append(candidate.Providers, ProviderEntry{
					ID: fmt.Sprintf("provider-%d", i),
				})
				return nil
			})
			if err != nil {
				t.Errorf("Transaction: %v", err)
			}
		}()
	}
	wait.Wait()
	if len(cfg.Providers) != count {
		t.Fatalf("providers = %d, want %d", len(cfg.Providers), count)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk Config
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatal(err)
	}
	if len(disk.Providers) != count {
		t.Fatalf("disk providers = %d, want %d", len(disk.Providers), count)
	}
}

func TestTransactionPreservesUnchangedSecretRef(t *testing.T) {
	t.Setenv("ZYHIVE_TEST_KEY", "resolved-secret")
	path := filepath.Join(t.TempDir(), "aipanel.json")
	raw := fmt.Sprintf(`{
		"gateway":{"port":8080,"bind":"localhost"},
		"agents":{"dir":"./agents"},
		"providers":[{"id":"p1","apiKey":%q}],
		"models":[],"channels":[],"tools":[],"skills":[],
		"auth":{"mode":"token","token":"plain-token"}
	}`, `{"$env":"ZYHIVE_TEST_KEY"}`)
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers[0].APIKey != "resolved-secret" {
		t.Fatal("secret reference was not resolved at runtime")
	}
	if err := Transaction(path, cfg, func(candidate *Config) error {
		candidate.Gateway.Port = 9090
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !containsBytes(data, []byte(`{\"$env\":\"ZYHIVE_TEST_KEY\"}`)) {
		t.Fatalf("secret reference was flattened: %s", data)
	}
}

func containsBytes(data, needle []byte) bool {
	for i := 0; i+len(needle) <= len(data); i++ {
		if string(data[i:i+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
