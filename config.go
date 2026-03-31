// Package sapmcpconfig provides shared configuration types for MCP servers
// that connect to SAP systems.
package sapmcpconfig

import (
	"encoding/json"
	"fmt"
	"os"
)

// SAPSystem describes a single SAP system's connection details and credentials.
type SAPSystem struct {
	Host           string `json:"host"`
	Client         string `json:"client"`
	User           string `json:"user,omitempty"`
	Password       string `json:"password,omitempty"`
	Language       string `json:"language,omitempty"`
	TLSSkipVerify  bool   `json:"tls_skip_verify,omitempty"`
	OAuth2ClientID string `json:"oauth2_client_id,omitempty"`
}

// IsOAuth2 returns true when the system is configured for OAuth2 (no user/password).
func (s SAPSystem) IsOAuth2() bool {
	return s.User == "" && s.Password == ""
}

// Config holds all configured SAP systems and a default system name.
type Config struct {
	DefaultSystem string               `json:"default_system"`
	Systems       map[string]SAPSystem `json:"systems"`
}

// Load reads a Config from the given JSON file and validates it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}
	return Parse(data)
}

// Parse unmarshals JSON bytes into a Config and validates it.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config (expected JSON): %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate checks that the Config is well-formed.
func (c *Config) Validate() error {
	if len(c.Systems) == 0 {
		return fmt.Errorf("config has no systems defined")
	}
	if _, ok := c.Systems[c.DefaultSystem]; !ok {
		return fmt.Errorf("default_system %q not found in systems", c.DefaultSystem)
	}
	for name, sys := range c.Systems {
		if sys.Host == "" {
			return fmt.Errorf("system %q has no host configured", name)
		}
		if (sys.User == "") != (sys.Password == "") {
			return fmt.Errorf("system %q: must have both user and password, or neither (for OAuth2)", name)
		}
	}
	return nil
}
