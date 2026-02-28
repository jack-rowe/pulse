package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Slack sends notifications via Slack incoming webhooks.
type Slack struct {
	webhookURL string
	client     *http.Client
}

// NewSlack creates a Slack notifier from a webhook URL.
func NewSlack(webhookURL string) *Slack {
	return &Slack{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *Slack) Name() string { return "slack" }

func (s *Slack) Notify(event Event) error {
	payload := map[string]string{
		"text": FormatMessage(event),
	}
	body, _ := json.Marshal(payload)

	resp, err := s.client.Post(s.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}
	return nil
}
