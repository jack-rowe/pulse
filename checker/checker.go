// Package checker defines the health check interface and result types.
package checker

import (
	"context"
	"time"
)

// Status represents the result of a health check.
type Status int

const (
	StatusUp   Status = iota // Check succeeded
	StatusDown               // Check failed
)

func (s Status) String() string {
	switch s {
	case StatusUp:
		return "up"
	case StatusDown:
		return "down"
	default:
		return "unknown"
	}
}

// Result holds the outcome of a single health check execution.
type Result struct {
	Status     Status        `json:"status"`
	StatusCode int           `json:"status_code,omitempty"` // HTTP status code if applicable
	Latency    time.Duration `json:"latency_ms"`
	Error      string        `json:"error,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
}

// Checker is the interface that all health check implementations must satisfy.
type Checker interface {
	// Check performs the health check and returns the result.
	// The context carries the deadline/timeout and cancellation signal.
	Check(ctx context.Context) Result
}
