package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const defaultConfigPath = "/usr/local/etc/mcpd.conf"

// Config represents the TOML configuration for mcpd.
type Config struct {
	Auth AuthConfig `toml:"auth"`
}

// AuthConfig holds authentication settings for mcpd.
type AuthConfig struct {
	APIKey string `toml:"api_key"`
}

func generateAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random api key: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func writeConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

// loadOrCreateConfig loads the TOML configuration from path, creating the file
// and/or generating auth.api_key if either is missing.
func loadOrCreateConfig(path string) (Config, bool, error) {
	var cfg Config

	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Config{}, false, fmt.Errorf("stat config file: %w", err)
		}

		key, err := generateAPIKey()
		if err != nil {
			return Config{}, false, err
		}
		cfg.Auth.APIKey = key
		if err := writeConfig(path, cfg); err != nil {
			return Config{}, false, err
		}
		return cfg, true, nil
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parse config file: %w", err)
	}

	cfg.Auth.APIKey = strings.TrimSpace(cfg.Auth.APIKey)
	if cfg.Auth.APIKey == "" {
		key, err := generateAPIKey()
		if err != nil {
			return Config{}, false, err
		}
		cfg.Auth.APIKey = key
		if err := writeConfig(path, cfg); err != nil {
			return Config{}, false, err
		}
		return cfg, true, nil
	}

	return cfg, false, nil
}
