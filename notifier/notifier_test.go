package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jack-rowe/pulse/checker"
)

func testEvent(status checker.Status) Event {
	return Event{
		EndpointName: "Test API",
		PrevStatus:   checker.StatusUp,
		NewStatus:    status,
		Error:        "connection refused",
		LatencyMs:    42.5,
		Timestamp:    time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
	}
}

// --- FormatMessage tests ---

func TestFormatMessageDown(t *testing.T) {
	msg := FormatMessage(testEvent(checker.StatusDown))

	if !strings.Contains(msg, "DOWN") {
		t.Error("down message should contain DOWN")
	}
	if !strings.Contains(msg, "Test API") {
		t.Error("down message should contain endpoint name")
	}
	if !strings.Contains(msg, "connection refused") {
		t.Error("down message should contain error")
	}
}

func TestFormatMessageUp(t *testing.T) {
	msg := FormatMessage(testEvent(checker.StatusUp))

	if !strings.Contains(msg, "UP") {
		t.Error("up message should contain UP")
	}
	if !strings.Contains(msg, "Test API") {
		t.Error("up message should contain endpoint name")
	}
	if !strings.Contains(msg, "42") {
		t.Error("up message should contain latency")
	}
}

// --- Log notifier tests ---

func TestLogNotifier(t *testing.T) {
	l := NewLog()

	if l.Name() != "log" {
		t.Errorf("expected name 'log', got %q", l.Name())
	}

	// Should not error
	if err := l.Notify(testEvent(checker.StatusDown)); err != nil {
		t.Errorf("log notify should not error: %v", err)
	}
	if err := l.Notify(testEvent(checker.StatusUp)); err != nil {
		t.Errorf("log notify should not error: %v", err)
	}
}

// --- Multi notifier tests ---

type mockNotifier struct {
	name   string
	events []Event
	err    error
}

func (m *mockNotifier) Name() string { return m.name }
func (m *mockNotifier) Notify(e Event) error {
	m.events = append(m.events, e)
	return m.err
}

func TestMultiFanOut(t *testing.T) {
	n1 := &mockNotifier{name: "a"}
	n2 := &mockNotifier{name: "b"}
	multi := NewMulti(n1, n2)

	if multi.Name() != "multi" {
		t.Errorf("expected name 'multi', got %q", multi.Name())
	}

	event := testEvent(checker.StatusDown)
	if err := multi.Notify(event); err != nil {
		t.Errorf("expected no error: %v", err)
	}

	if len(n1.events) != 1 {
		t.Errorf("n1 should have 1 event, got %d", len(n1.events))
	}
	if len(n2.events) != 1 {
		t.Errorf("n2 should have 1 event, got %d", len(n2.events))
	}
}

func TestMultiContinuesOnError(t *testing.T) {
	failing := &mockNotifier{name: "fail", err: io.ErrUnexpectedEOF}
	ok := &mockNotifier{name: "ok"}
	multi := NewMulti(failing, ok)

	event := testEvent(checker.StatusDown)
	err := multi.Notify(event)

	// Should still deliver to ok notifier despite failing one
	if len(ok.events) != 1 {
		t.Error("ok notifier should still receive event after prior failure")
	}
	if err == nil {
		t.Error("multi should return error when a notifier fails")
	}
}

// --- Slack notifier tests ---

func TestSlackNotify(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)

		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected json content type")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	slack := NewSlack(srv.URL)
	if slack.Name() != "slack" {
		t.Errorf("expected name 'slack', got %q", slack.Name())
	}

	err := slack.Notify(testEvent(checker.StatusDown))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]string
	json.Unmarshal([]byte(receivedBody), &payload)
	if payload["text"] == "" {
		t.Error("slack payload should have text field")
	}
}

func TestSlackNotifyServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	slack := NewSlack(srv.URL)
	err := slack.Notify(testEvent(checker.StatusDown))
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

// --- Discord notifier tests ---

func TestDiscordNotify(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	discord := NewDiscord(srv.URL)
	if discord.Name() != "discord" {
		t.Errorf("expected name 'discord', got %q", discord.Name())
	}

	err := discord.Notify(testEvent(checker.StatusDown))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]string
	json.Unmarshal([]byte(receivedBody), &payload)
	if payload["content"] == "" {
		t.Error("discord payload should have content field")
	}
}

func TestDiscordNotifyServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	discord := NewDiscord(srv.URL)
	err := discord.Notify(testEvent(checker.StatusDown))
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

// --- Webhook notifier tests ---

func TestWebhookNotify(t *testing.T) {
	var receivedHeaders http.Header
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		receivedBody, _ = io.ReadAll(r.Body)

		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	headers := map[string]string{"Authorization": "Bearer test-token"}
	wh := NewWebhook(srv.URL, headers)

	if wh.Name() != "webhook" {
		t.Errorf("expected name 'webhook', got %q", wh.Name())
	}

	err := wh.Notify(testEvent(checker.StatusDown))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedHeaders.Get("Authorization") != "Bearer test-token" {
		t.Error("custom header not received")
	}
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Error("expected json content type")
	}
	if receivedHeaders.Get("User-Agent") != "Pulse/1.0" {
		t.Error("expected Pulse user agent")
	}

	var event Event
	if err := json.Unmarshal(receivedBody, &event); err != nil {
		t.Fatalf("webhook body should be a valid Event JSON: %v", err)
	}
	if event.EndpointName != "Test API" {
		t.Errorf("expected endpoint 'Test API', got %q", event.EndpointName)
	}
}

func TestWebhookNotifyServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	wh := NewWebhook(srv.URL, nil)
	err := wh.Notify(testEvent(checker.StatusDown))
	if err == nil {
		t.Error("expected error for non-2xx response")
	}
}

func TestWebhookNotifyConnectionError(t *testing.T) {
	wh := NewWebhook("http://127.0.0.1:1", nil)
	err := wh.Notify(testEvent(checker.StatusDown))
	if err == nil {
		t.Error("expected error for connection failure")
	}
}
