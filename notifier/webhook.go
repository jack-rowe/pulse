package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Webhook sends notifications via generic HTTP POST.
type Webhook struct {
	url     string
	headers map[string]string
	client  *http.Client
}

// NewWebhook creates a generic webhook notifier.
func NewWebhook(url string, headers map[string]string) *Webhook {
	return &Webhook{
		url:     url,
		headers: headers,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *Webhook) Name() string { return "webhook" }

func (w *Webhook) Notify(event Event) error {
	body, _ := json.Marshal(event)

	req, err := http.NewRequest("POST", w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Pulse/1.0")
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
