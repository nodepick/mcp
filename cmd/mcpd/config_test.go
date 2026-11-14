package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateConfig_CreatesNewFile(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "nested", "mcpd.conf")

	cfg, created, err := loadOrCreateConfig(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !created {
		t.Fatalf("expected created=true for new config file")
	}
	if len(cfg.Auth.APIKey) != 64 {
		t.Fatalf("expected 64-char hex api_key, got %q (len %d)", cfg.Auth.APIKey, len(cfg.Auth.APIKey))
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected config file permissions 0600, got %o", perm)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read generated config: %v", err)
	}
	content := string(raw)
	expected := "[auth]\napi_key = \"" + cfg.Auth.APIKey + "\"\n"
	if content != expected {
		t.Errorf("expected TOML content:\n%q\ngot:\n%q", expected, content)
	}

	// Subsequent load should reuse the exact same key without recreating.
	cfg2, created2, err := loadOrCreateConfig(cfgPath)
	if err != nil {
		t.Fatalf("expected no error on reload, got %v", err)
	}
	if created2 {
		t.Fatalf("expected created=false on reload")
	}
	if cfg2.Auth.APIKey != cfg.Auth.APIKey {
		t.Errorf("expected reloaded key %q, got %q", cfg.Auth.APIKey, cfg2.Auth.APIKey)
	}
}

func TestLoadOrCreateConfig_LoadsExistingKey(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "mcpd.conf")
	existing := "[auth]\napi_key = \"my-custom-secret\"\n"
	if err := os.WriteFile(cfgPath, []byte(existing), 0600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	cfg, created, err := loadOrCreateConfig(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created {
		t.Fatalf("expected created=false for existing valid config")
	}
	if cfg.Auth.APIKey != "my-custom-secret" {
		t.Errorf("expected 'my-custom-secret', got %q", cfg.Auth.APIKey)
	}
}

func TestLoadOrCreateConfig_GeneratesKeyWhenMissingInExistingFile(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "mcpd.conf")
	if err := os.WriteFile(cfgPath, []byte("[auth]\napi_key = \"\"\n"), 0600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	cfg, created, err := loadOrCreateConfig(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !created {
		t.Fatalf("expected created=true when existing config has empty api_key")
	}
	if len(cfg.Auth.APIKey) != 64 {
		t.Fatalf("expected 64-char hex api_key, got %q", cfg.Auth.APIKey)
	}
}

func TestLoadOrCreateConfig_InvalidTOML(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "mcpd.conf")
	if err := os.WriteFile(cfgPath, []byte("[auth\nbad toml"), 0600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	if _, _, err := loadOrCreateConfig(cfgPath); err == nil {
		t.Fatalf("expected error for invalid TOML, got nil")
	}
}
