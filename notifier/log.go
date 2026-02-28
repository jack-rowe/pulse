package notifier

import (
	"log/slog"
)

// Log is a notifier that writes to stdout via slog. Always enabled as a fallback.
type Log struct{}

// NewLog creates a log notifier.
func NewLog() *Log {
	return &Log{}
}

func (l *Log) Name() string { return "log" }

func (l *Log) Notify(event Event) error {
	msg := FormatMessage(event)
	if event.NewStatus == 0 { // StatusUp
		slog.Info("endpoint recovered", "endpoint", event.EndpointName, "message", msg)
	} else {
		slog.Warn("endpoint down", "endpoint", event.EndpointName, "message", msg)
	}
	return nil
}
