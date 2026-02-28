// Package scheduler orchestrates periodic health checks across all configured endpoints.
package scheduler

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jack-rowe/pulse/checker"
	"github.com/jack-rowe/pulse/notifier"
	"github.com/jack-rowe/pulse/store"
)

// Target represents a single monitored endpoint with its checker and schedule.
type Target struct {
	Name          string
	Interval      time.Duration
	FailThreshold int // Consecutive failures required before alerting
	Checker       checker.Checker
}

// Scheduler runs health checks on configured intervals and handles state transitions.
type Scheduler struct {
	store    store.Store
	notifier notifier.Notifier

	mu     sync.RWMutex
	states map[string]*endpointState // name -> state
}

type endpointState struct {
	currentStatus    checker.Status
	consecutiveFails int
	alerted          bool // whether we've already sent a DOWN alert
}

// New creates a Scheduler.
func New(store store.Store, notifier notifier.Notifier) *Scheduler {
	return &Scheduler{
		store:    store,
		notifier: notifier,
		states:   make(map[string]*endpointState),
	}
}

// Start launches a goroutine per target. Each goroutine ticks on the target's interval.
// Blocks until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context, targets []Target) {
	var wg sync.WaitGroup

	for _, t := range targets {
		wg.Add(1)
		go func(target Target) {
			defer wg.Done()
			s.runTarget(ctx, target)
		}(t)
	}

	// Block until all goroutines finish (context cancellation)
	wg.Wait()
}

// StartAsync launches checks in background and returns immediately.
func (s *Scheduler) StartAsync(ctx context.Context, targets []Target) {
	for _, t := range targets {
		go s.runTarget(ctx, t)
	}
}

func (s *Scheduler) runTarget(ctx context.Context, target Target) {
	// Jitter startup so all checks don't fire at the exact same moment
	jitter := time.Duration(rand.Int64N(int64(target.Interval / 2)))
	select {
	case <-time.After(jitter):
	case <-ctx.Done():
		return
	}

	// Run first check immediately
	s.executeCheck(ctx, target)

	ticker := time.NewTicker(target.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("stopping checker", "endpoint", target.Name)
			return
		case <-ticker.C:
			s.executeCheck(ctx, target)
		}
	}
}

func (s *Scheduler) executeCheck(ctx context.Context, target Target) {
	result := target.Checker.Check(ctx)

	// Persist result
	record := store.CheckRecord{
		EndpointName: target.Name,
		Status:       result.Status,
		StatusCode:   result.StatusCode,
		LatencyMs:    float64(result.Latency.Milliseconds()),
		Error:        result.Error,
		Timestamp:    result.Timestamp,
	}

	if err := s.store.SaveResult(record); err != nil {
		slog.Error("failed to save result", "endpoint", target.Name, "error", err)
	}

	slog.Debug("check completed",
		"endpoint", target.Name,
		"status", result.Status,
		"latency_ms", result.Latency.Milliseconds(),
		"error", result.Error,
	)

	// Handle state transitions
	s.handleStateChange(target, result)
}

func (s *Scheduler) handleStateChange(target Target, result checker.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.states[target.Name]
	if !exists {
		state = &endpointState{currentStatus: checker.StatusUp}
		s.states[target.Name] = state
	}

	threshold := target.FailThreshold
	if threshold <= 0 {
		threshold = 3
	}

	switch result.Status {
	case checker.StatusDown:
		state.consecutiveFails++

		// Only alert once after hitting threshold
		if state.consecutiveFails >= threshold && !state.alerted {
			state.alerted = true
			state.currentStatus = checker.StatusDown

			slog.Warn("endpoint DOWN", "endpoint", target.Name, "consecutive_failures", state.consecutiveFails)

			event := notifier.Event{
				EndpointName: target.Name,
				PrevStatus:   checker.StatusUp,
				NewStatus:    checker.StatusDown,
				Error:        result.Error,
				LatencyMs:    float64(result.Latency.Milliseconds()),
				Timestamp:    result.Timestamp,
			}
			if err := s.notifier.Notify(event); err != nil {
				slog.Error("failed to send notification", "endpoint", target.Name, "error", err)
			}
		}

	case checker.StatusUp:
		// Recovery: was previously down and alerted
		if state.alerted {
			slog.Info("endpoint RECOVERED", "endpoint", target.Name)

			event := notifier.Event{
				EndpointName: target.Name,
				PrevStatus:   checker.StatusDown,
				NewStatus:    checker.StatusUp,
				LatencyMs:    float64(result.Latency.Milliseconds()),
				Timestamp:    result.Timestamp,
			}
			if err := s.notifier.Notify(event); err != nil {
				slog.Error("failed to send recovery notification", "endpoint", target.Name, "error", err)
			}
		}

		state.consecutiveFails = 0
		state.alerted = false
		state.currentStatus = checker.StatusUp
	}
}

// GetState returns the current state of all endpoints (for the API layer).
func (s *Scheduler) GetState() map[string]checker.Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]checker.Status, len(s.states))
	for name, state := range s.states {
		out[name] = state.currentStatus
	}
	return out
}
