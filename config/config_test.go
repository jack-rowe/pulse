package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
server:
  port: 9090
endpoints:
  - name: "Test API"
    type: http
    url: "https://example.com/health"
    expected_status: 200
    timeout_ms: 5000
    interval_sec: 30
storage:
  path: "test.db"
  retention_days: 30
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if len(cfg.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(cfg.Endpoints))
	}
	if cfg.Endpoints[0].Name != "Test API" {
		t.Errorf("expected name 'Test API', got %q", cfg.Endpoints[0].Name)
	}
	if cfg.Storage.Path != "test.db" {
		t.Errorf("expected storage path 'test.db', got %q", cfg.Storage.Path)
	}
	if cfg.Storage.RetentionDays != 30 {
		t.Errorf("expected retention 30, got %d", cfg.Storage.RetentionDays)
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{
		Endpoints: []Endpoint{
			{Name: "test", Type: "http", URL: "https://example.com"},
		},
	}
	applyDefaults(cfg)

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Storage.Path != "pulse.db" {
		t.Errorf("expected default storage path 'pulse.db', got %q", cfg.Storage.Path)
	}
	if cfg.Storage.RetentionDays != 90 {
		t.Errorf("expected default retention 90, got %d", cfg.Storage.RetentionDays)
	}
	ep := cfg.Endpoints[0]
	if ep.Method != "GET" {
		t.Errorf("expected default method GET, got %q", ep.Method)
	}
	if ep.ExpectedStatus != 200 {
		t.Errorf("expected default expected_status 200, got %d", ep.ExpectedStatus)
	}
	if ep.TimeoutMs != 10000 {
		t.Errorf("expected default timeout 10000, got %d", ep.TimeoutMs)
	}
	if ep.IntervalSec != 60 {
		t.Errorf("expected default interval 60, got %d", ep.IntervalSec)
	}
	if ep.FailThreshold != 3 {
		t.Errorf("expected default fail_threshold 3, got %d", ep.FailThreshold)
	}
}

func TestValidateNoEndpoints(t *testing.T) {
	cfg := &Config{}
	err := validate(cfg)
	if err == nil {
		t.Fatal("expected error for no endpoints")
	}
}

func TestValidateMissingName(t *testing.T) {
	cfg := &Config{
		Endpoints: []Endpoint{
			{Type: "http", URL: "https://example.com", IntervalSec: 30, TimeoutMs: 5000},
		},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidateDuplicateNames(t *testing.T) {
	cfg := &Config{
		Endpoints: []Endpoint{
			{Name: "api", Type: "http", URL: "https://a.com", IntervalSec: 30, TimeoutMs: 5000},
			{Name: "api", Type: "http", URL: "https://b.com", IntervalSec: 30, TimeoutMs: 5000},
		},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate names")
	}
}

func TestValidateUnknownType(t *testing.T) {
	cfg := &Config{
		Endpoints: []Endpoint{
			{Name: "test", Type: "grpc", URL: "https://example.com", IntervalSec: 30, TimeoutMs: 5000},
		},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestValidateTCPMissingAddress(t *testing.T) {
	cfg := &Config{
		Endpoints: []Endpoint{
			{Name: "db", Type: "tcp", IntervalSec: 30, TimeoutMs: 5000},
		},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("expected error for tcp with no address")
	}
}

func TestValidateHTTPMissingURL(t *testing.T) {
	cfg := &Config{
		Endpoints: []Endpoint{
			{Name: "api", Type: "http", IntervalSec: 30, TimeoutMs: 5000},
		},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("expected error for http with no url")
	}
}

func TestValidateIntervalTooLow(t *testing.T) {
	cfg := &Config{
		Endpoints: []Endpoint{
			{Name: "api", Type: "http", URL: "https://example.com", IntervalSec: 2, TimeoutMs: 5000},
		},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("expected error for interval < 5")
	}
}

func TestValidateTimeoutTooLow(t *testing.T) {
	cfg := &Config{
		Endpoints: []Endpoint{
			{Name: "api", Type: "http", URL: "https://example.com", IntervalSec: 30, TimeoutMs: 50},
		},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("expected error for timeout < 100")
	}
}

func TestValidateInvalidName(t *testing.T) {
	cfg := &Config{
		Endpoints: []Endpoint{
			{Name: "!@#bad", Type: "http", URL: "https://example.com", IntervalSec: 30, TimeoutMs: 5000},
		},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid name chars")
	}
}

func TestValidateValidConfig(t *testing.T) {
	cfg := &Config{
		Endpoints: []Endpoint{
			{Name: "API Server", Type: "http", URL: "https://example.com", IntervalSec: 30, TimeoutMs: 5000},
			{Name: "DB", Type: "tcp", Address: "db:5432", IntervalSec: 30, TimeoutMs: 5000},
			{Name: "WS", Type: "websocket", URL: "wss://example.com/ws", IntervalSec: 15, TimeoutMs: 5000},
		},
	}
	err := validate(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestExpandEnvVars(t *testing.T) {
	t.Setenv("PULSE_TEST_SECRET", "my-secret-key")

	input := []byte(`api_key: "${PULSE_TEST_SECRET}"`)
	result := expandEnvVars(input)

	expected := `api_key: "my-secret-key"`
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestExpandEnvVarsUnset(t *testing.T) {
	input := []byte(`api_key: "${NONEXISTENT_VAR_ABC123}"`)
	result := expandEnvVars(input)

	// Unresolved vars should be left as-is
	if string(result) != string(input) {
		t.Errorf("expected unresolved var to be left as-is, got %q", string(result))
	}
}

func TestEndpointTimeout(t *testing.T) {
	ep := Endpoint{TimeoutMs: 5000}
	if ep.Timeout().Milliseconds() != 5000 {
		t.Errorf("expected 5000ms, got %d", ep.Timeout().Milliseconds())
	}

	ep2 := Endpoint{TimeoutMs: 0}
	if ep2.Timeout().Seconds() != 10 {
		t.Errorf("expected 10s default, got %v", ep2.Timeout())
	}
}

func TestEndpointInterval(t *testing.T) {
	ep := Endpoint{IntervalSec: 30}
	if ep.Interval().Seconds() != 30 {
		t.Errorf("expected 30s, got %v", ep.Interval())
	}

	ep2 := Endpoint{IntervalSec: 0}
	if ep2.Interval().Seconds() != 60 {
		t.Errorf("expected 60s default, got %v", ep2.Interval())
	}
}

func TestGenerateDefault(t *testing.T) {
	yaml := GenerateDefault()
	if len(yaml) == 0 {
		t.Fatal("GenerateDefault returned empty string")
	}
	// Should be parseable
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("generated default config should be valid: %v", err)
	}
	if len(cfg.Endpoints) == 0 {
		t.Error("generated config should have endpoints")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error loading missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{{{not yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
