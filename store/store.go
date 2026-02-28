// Package store defines the persistence interface and data types for check results.
package store

import (
	"time"

	"github.com/jack-rowe/pulse/checker"
)

// CheckRecord is a persisted health check result.
type CheckRecord struct {
	EndpointName string         `json:"endpoint_name"`
	Status       checker.Status `json:"status"`
	StatusCode   int            `json:"status_code,omitempty"`
	LatencyMs    float64        `json:"latency_ms"`
	Error        string         `json:"error,omitempty"`
	Timestamp    time.Time      `json:"timestamp"`
}

// UptimeSummary holds computed uptime statistics for an endpoint.
type UptimeSummary struct {
	EndpointName  string         `json:"endpoint_name"`
	CurrentStatus checker.Status `json:"current_status"`
	UptimePercent float64        `json:"uptime_percent"`
	AvgLatencyMs  float64        `json:"avg_latency_ms"`
	MinLatencyMs  float64        `json:"min_latency_ms"`
	MaxLatencyMs  float64        `json:"max_latency_ms"`
	TotalChecks   int            `json:"total_checks"`
	TotalFailures int            `json:"total_failures"`
	LastChecked   time.Time      `json:"last_checked"`
	LastDownAt    *time.Time     `json:"last_down_at,omitempty"`
}

// TimelineBucket represents a time window with aggregated check results.
type TimelineBucket struct {
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	TotalChecks  int       `json:"total_checks"`
	Failures     int       `json:"failures"`
	AvgLatencyMs float64   `json:"avg_latency_ms"`
	MaxLatencyMs float64   `json:"max_latency_ms"`
	Status       string    `json:"status"` // "up", "down", "degraded", "empty"
}

// Store is the interface for persisting and querying check results.
type Store interface {
	// SaveResult persists a single check result.
	SaveResult(record CheckRecord) error

	// GetLatest returns the most recent result for each endpoint.
	GetLatest() ([]CheckRecord, error)

	// GetHistory returns check results for a specific endpoint, ordered by time descending.
	// limit controls max records returned. Use 0 for all records.
	GetHistory(endpointName string, limit int) ([]CheckRecord, error)

	// GetUptimeSummary returns uptime statistics for an endpoint over the given duration.
	GetUptimeSummary(endpointName string, duration time.Duration) (*UptimeSummary, error)

	// GetTimeline returns bucketed check results for the timeline visualization.
	// duration is the total time span, buckets is how many segments to divide it into.
	GetTimeline(endpointName string, duration time.Duration, buckets int) ([]TimelineBucket, error)

	// PurgeOlderThan deletes records older than the given time.
	PurgeOlderThan(before time.Time) (int, error)

	// Close releases all resources.
	Close() error
}
