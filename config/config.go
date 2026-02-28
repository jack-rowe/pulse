package config

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for Pulse.
type Config struct {
	Server    ServerConfig   `yaml:"server"`
	Endpoints []Endpoint     `yaml:"endpoints"`
	Alerting  AlertingConfig `yaml:"alerting"`
	Storage   StorageConfig  `yaml:"storage"`
}

// ServerConfig controls the built-in HTTP status API.
type ServerConfig struct {
	Port   int    `yaml:"port"`
	APIKey string `yaml:"api_key"`
}

// Endpoint defines a single target to monitor.
type Endpoint struct {
	Name           string            `yaml:"name"`
	Type           string            `yaml:"type"` // "http", "tcp", "websocket"
	URL            string            `yaml:"url,omitempty"`
	Address        string            `yaml:"address,omitempty"` // host:port for TCP checks
	Method         string            `yaml:"method,omitempty"`  // GET, HEAD, POST (default: GET)
	ExpectedStatus int               `yaml:"expected_status,omitempty"`
	ExpectedBody   string            `yaml:"expected_body,omitempty"` // substring match
	TimeoutMs      int               `yaml:"timeout_ms"`
	IntervalSec    int               `yaml:"interval_sec"`
	Headers        map[string]string `yaml:"headers,omitempty"`
	FailThreshold  int               `yaml:"fail_threshold,omitempty"` // consecutive failures before alert (default: 3)
}

// Timeout returns the timeout as a time.Duration.
func (e Endpoint) Timeout() time.Duration {
	if e.TimeoutMs <= 0 {
		return 10 * time.Second
	}
	return time.Duration(e.TimeoutMs) * time.Millisecond
}

// Interval returns the check interval as a time.Duration.
func (e Endpoint) Interval() time.Duration {
	if e.IntervalSec <= 0 {
		return 60 * time.Second
	}
	return time.Duration(e.IntervalSec) * time.Second
}

// AlertingConfig holds notification channel configurations.
type AlertingConfig struct {
	Slack   SlackConfig   `yaml:"slack,omitempty"`
	Discord DiscordConfig `yaml:"discord,omitempty"`
	Webhook WebhookConfig `yaml:"webhook,omitempty"`
	SMTP    SMTPConfig    `yaml:"smtp,omitempty"`
}

type SlackConfig struct {
	WebhookURL string `yaml:"webhook_url"`
}

type DiscordConfig struct {
	WebhookURL string `yaml:"webhook_url"`
}

type WebhookConfig struct {
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

type SMTPConfig struct {
	Host     string   `yaml:"host"`
	Port     int      `yaml:"port"`
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
	From     string   `yaml:"from"`
	To       []string `yaml:"to"`
}

// StorageConfig controls the embedded data store.
type StorageConfig struct {
	Path          string `yaml:"path"`           // database file path (default: pulse.db)
	RetentionDays int    `yaml:"retention_days"` // auto-cleanup older than N days (default: 90)
}

// Load reads and parses a YAML config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Storage.Path == "" {
		cfg.Storage.Path = "pulse.db"
	}
	if cfg.Storage.RetentionDays == 0 {
		cfg.Storage.RetentionDays = 90
	}
	for i := range cfg.Endpoints {
		if cfg.Endpoints[i].Method == "" {
			cfg.Endpoints[i].Method = "GET"
		}
		if cfg.Endpoints[i].ExpectedStatus == 0 && cfg.Endpoints[i].Type == "http" {
			cfg.Endpoints[i].ExpectedStatus = 200
		}
		if cfg.Endpoints[i].TimeoutMs == 0 {
			cfg.Endpoints[i].TimeoutMs = 10000
		}
		if cfg.Endpoints[i].IntervalSec == 0 {
			cfg.Endpoints[i].IntervalSec = 60
		}
		if cfg.Endpoints[i].FailThreshold == 0 {
			cfg.Endpoints[i].FailThreshold = 3
		}
	}
}

func validate(cfg *Config) error {
	if len(cfg.Endpoints) == 0 {
		return fmt.Errorf("at least one endpoint is required")
	}

	seen := make(map[string]bool)
	for i, ep := range cfg.Endpoints {
		if ep.Name == "" {
			return fmt.Errorf("endpoint %d: name is required", i)
		}
		if seen[ep.Name] {
			return fmt.Errorf("endpoint %d: duplicate name %q", i, ep.Name)
		}
		seen[ep.Name] = true

		switch ep.Type {
		case "http", "websocket":
			if ep.URL == "" {
				return fmt.Errorf("endpoint %q: url is required for type %q", ep.Name, ep.Type)
			}
			if _, err := url.Parse(ep.URL); err != nil {
				return fmt.Errorf("endpoint %q: invalid url: %w", ep.Name, err)
			}
		case "tcp":
			if ep.Address == "" {
				return fmt.Errorf("endpoint %q: address is required for type tcp", ep.Name)
			}
		default:
			return fmt.Errorf("endpoint %q: unknown type %q (must be http, tcp, or websocket)", ep.Name, ep.Type)
		}

		if ep.IntervalSec < 5 {
			return fmt.Errorf("endpoint %q: interval_sec must be >= 5", ep.Name)
		}
		if ep.TimeoutMs < 100 {
			return fmt.Errorf("endpoint %q: timeout_ms must be >= 100", ep.Name)
		}
	}

	return nil
}

// GenerateDefault returns a default config YAML string for --init.
func GenerateDefault() string {
	return `# Pulse — Uptime Monitor Configuration
# Documentation: https://github.com/jack-rowe/pulse

server:
  port: 8080
  # api_key: "your-secret-key"  # Uncomment to require auth on API

endpoints:
  - name: "Production API"
    type: http
    url: "https://api.example.com/health"
    expected_status: 200
    timeout_ms: 5000
    interval_sec: 30
    fail_threshold: 3

  - name: "Website"
    type: http
    url: "https://www.example.com"
    expected_status: 200
    timeout_ms: 5000
    interval_sec: 60

  # - name: "Realtime Server"
  #   type: websocket
  #   url: "wss://rt.example.com/ws"
  #   timeout_ms: 5000
  #   interval_sec: 15

  # - name: "Database"
  #   type: tcp
  #   address: "db.internal:5432"
  #   timeout_ms: 3000
  #   interval_sec: 30

alerting:
  slack:
    webhook_url: ""
  # discord:
  #   webhook_url: ""
  # webhook:
  #   url: "https://hooks.example.com/custom"
  #   headers:
  #     Authorization: "Bearer token"
  # smtp:
  #   host: "smtp.gmail.com"
  #   port: 587
  #   username: "alerts@example.com"
  #   password: "app-password"
  #   from: "alerts@example.com"
  #   to:
  #     - "oncall@example.com"

storage:
  path: "pulse.db"
  retention_days: 90
`
}
