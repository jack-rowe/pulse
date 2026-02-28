package notifier

import (
	"log/slog"
)

// Multi fans out notifications to multiple notifiers.
type Multi struct {
	notifiers []Notifier
}

// NewMulti creates a multi-notifier that sends to all provided notifiers.
func NewMulti(notifiers ...Notifier) *Multi {
	return &Multi{notifiers: notifiers}
}

func (m *Multi) Name() string { return "multi" }

// Notify sends to all notifiers. Logs errors but does not fail on partial errors,
// so one broken channel doesn't block the others.
func (m *Multi) Notify(event Event) error {
	var lastErr error
	for _, n := range m.notifiers {
		if err := n.Notify(event); err != nil {
			slog.Error("notifier failed", "notifier", n.Name(), "error", err)
			lastErr = err
		}
	}
	return lastErr
}
