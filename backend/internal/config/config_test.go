// Package config tests for configuration loading and validation.
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
ntp:
  enabled: true
  port: 123
  stratum: 2
  poll_interval: 64s
  upstream_peers:
    - address: "time.cloudflare.com:123"
      prefer: true

nts:
  enabled: false

ptp:
  enabled: false

gpsdo:
  enabled: false

web:
  port: 8080
  jwt_secret: "test-secret"
  users:
    - username: "admin"
      password_hash: "$2a$10$test"
      role: "admin"

logging:
  level: "info"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify NTP config
	if !cfg.NTP.Enabled {
		t.Error("NTP should be enabled")
	}
	if cfg.NTP.Port != 123 {
		t.Errorf("NTP port should be 123, got %d", cfg.NTP.Port)
	}
	if cfg.NTP.Stratum != 2 {
		t.Errorf("NTP stratum should be 2, got %d", cfg.NTP.Stratum)
	}
	if len(cfg.NTP.UpstreamPeers) != 1 {
		t.Errorf("Expected 1 upstream peer, got %d", len(cfg.NTP.UpstreamPeers))
	}

	// Verify Web config
	if cfg.Web.Port != 8080 {
		t.Errorf("Web port should be 8080, got %d", cfg.Web.Port)
	}
	if len(cfg.Web.Users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(cfg.Web.Users))
	}
}

func TestConfigDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Minimal config - should get defaults
	configContent := `
ntp:
  enabled: true
web:
  jwt_secret: "test"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check defaults were applied
	if cfg.NTP.Port != 123 {
		t.Errorf("Default NTP port should be 123, got %d", cfg.NTP.Port)
	}
	if cfg.NTP.Stratum != 2 {
		t.Errorf("Default NTP stratum should be 2, got %d", cfg.NTP.Stratum)
	}
	if cfg.NTP.PollInterval != 64*time.Second {
		t.Errorf("Default poll interval should be 64s, got %v", cfg.NTP.PollInterval)
	}
	if cfg.Web.Port != 8080 {
		t.Errorf("Default web port should be 8080, got %d", cfg.Web.Port)
	}
	if cfg.PTP.Domain != 0 {
		t.Errorf("Default PTP domain should be 0, got %d", cfg.PTP.Domain)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			name: "valid minimal config",
			config: `
ntp:
  enabled: true
web:
  jwt_secret: "test"
`,
			wantErr: false,
		},
		{
			name: "invalid NTP port",
			config: `
ntp:
  enabled: true
  port: 999999
web:
  jwt_secret: "test"
`,
			wantErr: true,
		},
		{
			name: "invalid NTP stratum",
			config: `
ntp:
  enabled: true
  stratum: 20
web:
  jwt_secret: "test"
`,
			wantErr: true,
		},
		{
			name: "NTS enabled without cert",
			config: `
ntp:
  enabled: true
nts:
  enabled: true
web:
  jwt_secret: "test"
`,
			wantErr: true,
		},
		{
			name: "PTP enabled without interface",
			config: `
ntp:
  enabled: true
ptp:
  enabled: true
web:
  jwt_secret: "test"
`,
			wantErr: true,
		},
		{
			name: "invalid GPSDO type",
			config: `
ntp:
  enabled: true
gpsdo:
  enabled: true
  type: "invalid"
web:
  jwt_secret: "test"
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			if err := os.WriteFile(configPath, []byte(tt.config), 0644); err != nil {
				t.Fatalf("Failed to write test config: %v", err)
			}

			_, err := Load(configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEnvironmentVariableExpansion(t *testing.T) {
	// Set environment variable
	os.Setenv("TEST_JWT_SECRET", "my-secret-from-env")
	defer os.Unsetenv("TEST_JWT_SECRET")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
ntp:
  enabled: true
web:
  jwt_secret: "${TEST_JWT_SECRET}"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Web.JWTSecret != "my-secret-from-env" {
		t.Errorf("JWT secret should be expanded from env, got %s", cfg.Web.JWTSecret)
	}
}
