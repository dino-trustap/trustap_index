package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type config struct {
	Logging loggingConfig `yaml:"logging"`
	Server  serverConfig  `yaml:"server"`

	// PublicBaseURL is the externally reachable base URL of this service,
	// used to build the redirect_uri given to the Trustap actions page.
	PublicBaseURL string `yaml:"public_base_url"`

	Database  databaseConfig   `yaml:"database"`
	Trustap   trustapConfig    `yaml:"trustap"`
	Merchants []merchantConfig `yaml:"merchants"`
}

type loggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type serverConfig struct {
	ReadTimeoutSeconds  int `yaml:"read_timeout_seconds"`
	WriteTimeoutSeconds int `yaml:"write_timeout_seconds"`
	IdleTimeoutSeconds  int `yaml:"idle_timeout_seconds"`
}

type databaseConfig struct {
	// DSN in lib/pq format, e.g.
	// "host=localhost port=5433 user=postgres password=postgres dbname=trustap_index sslmode=disable"
	DSN string `yaml:"dsn"`
}

type trustapConfig struct {
	// Environment selects the Trustap base URLs: "test" (stage) or "live".
	Environment string `yaml:"environment"`
	// WebhookUser/WebhookPassword protect POST /api/webhooks/trustap; the
	// same pair is entered in the Trustap internal dashboard.
	WebhookUser     string `yaml:"webhook_user"`
	WebhookPassword string `yaml:"webhook_password"`
}

type merchantConfig struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	// TrustapAPIKey is the merchant's own Trustap API client key
	// (per-merchant client model).
	TrustapAPIKey string `yaml:"trustap_api_key"`
	// TrustapSub is the merchant's Trustap user ID, granted to the client
	// via the one-time OAuth consent.
	TrustapSub string `yaml:"trustap_sub"`
}

func readConfig(path string) (*config, error) {
	rawConfig, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("couldn't read file: %w", err)
	}

	config := &config{}
	err = yaml.Unmarshal(rawConfig, config)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse YAML: %w", err)
	}

	return config, nil
}
