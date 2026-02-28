package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Discord sends notifications via Discord webhooks.
type Discord struct {
	webhookURL string
	client     *http.Client
}

// NewDiscord creates a Discord notifier.
func NewDiscord(webhookURL string) *Discord {
	return &Discord{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *Discord) Name() string { return "discord" }

func (d *Discord) Notify(event Event) error {
	payload := map[string]string{
		"content": FormatMessage(event),
	}
	body, _ := json.Marshal(payload)

	resp, err := d.client.Post(d.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned %d", resp.StatusCode)
	}
	return nil
}
