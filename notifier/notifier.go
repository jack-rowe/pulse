// Package notifier defines the alerting interface and implementations.
package notifier

import (
	"fmt"
	"time"

	"github.com/jack-rowe/pulse/checker"
)

// Event represents a state change that triggers a notification.
type Event struct {
	EndpointName string         `json:"endpoint_name"`
	PrevStatus   checker.Status `json:"prev_status"`
	NewStatus    checker.Status `json:"new_status"`
	Error        string         `json:"error,omitempty"`
	LatencyMs    float64        `json:"latency_ms"`
	Timestamp    time.Time      `json:"timestamp"`
}

// Notifier is the interface for sending alerts.
type Notifier interface {
	Notify(event Event) error
	Name() string
}

// FormatMessage returns a human-readable message for an event.
func FormatMessage(e Event) string {
	if e.NewStatus == checker.StatusDown {
		msg := fmt.Sprintf("🔴 DOWN — %s is unreachable", e.EndpointName)
		if e.Error != "" {
			msg += fmt.Sprintf("\nError: %s", e.Error)
		}
		msg += fmt.Sprintf("\nTime: %s", e.Timestamp.UTC().Format(time.RFC3339))
		return msg
	}
	return fmt.Sprintf("🟢 UP — %s has recovered\nLatency: %.0fms\nTime: %s",
		e.EndpointName, e.LatencyMs, e.Timestamp.UTC().Format(time.RFC3339))
}
