package scheduler

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jack-rowe/pulse/checker"
	"github.com/jack-rowe/pulse/notifier"
	"github.com/jack-rowe/pulse/store"
)

// --- mock checker ---

type mockChecker struct {
	mu      sync.Mutex
	results []checker.Result
	calls   int
}

func (m *mockChecker) Check(ctx context.Context) checker.Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.calls
	m.calls++
	if idx < len(m.results) {
		r := m.results[idx]
		r.Timestamp = time.Now()
		return r
	}
	// If we have results defined, repeat the last one; otherwise default to up
	if len(m.results) > 0 {
		r := m.results[len(m.results)-1]
		r.Timestamp = time.Now()
		return r
	}
	return checker.Result{Status: checker.StatusUp, Latency: time.Millisecond, Timestamp: time.Now()}
}

func (m *mockChecker) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// --- mock notifier ---

type mockNotifier struct {
	mu     sync.Mutex
	events []notifier.Event
}

func (m *mockNotifier) Name() string { return "mock" }
func (m *mockNotifier) Notify(e notifier.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, e)
	return nil
}
func (m *mockNotifier) Events() []notifier.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]notifier.Event, len(m.events))
	copy(cp, m.events)
	return cp
}

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := store.NewBolt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSchedulerExecutesChecks(t *testing.T) {
	db := newTestStore(t)
	n := &mockNotifier{}
	chk := &mockChecker{}

	sched := New(db, n)
	ctx, cancel := context.WithCancel(context.Background())

	targets := []Target{
		{
			Name:          "test-ep",
			Interval:      50 * time.Millisecond,
			FailThreshold: 3,
			Checker:       chk,
		},
	}

	sched.StartAsync(ctx, targets)

	// Wait for several check cycles
	time.Sleep(300 * time.Millisecond)
	cancel()

	if chk.CallCount() < 2 {
		t.Errorf("expected at least 2 checks, got %d", chk.CallCount())
	}

	// Verify results were persisted
	history, _ := db.GetHistory("test-ep", 0)
	if len(history) < 2 {
		t.Errorf("expected at least 2 persisted results, got %d", len(history))
	}
}

func TestSchedulerStateTransitionDownAlert(t *testing.T) {
	db := newTestStore(t)
	n := &mockNotifier{}

	// Return 3 consecutive failures to hit threshold
	chk := &mockChecker{
		results: []checker.Result{
			{Status: checker.StatusDown, Error: "fail 1", Latency: time.Millisecond},
			{Status: checker.StatusDown, Error: "fail 2", Latency: time.Millisecond},
			{Status: checker.StatusDown, Error: "fail 3", Latency: time.Millisecond},
		},
	}

	sched := New(db, n)
	ctx, cancel := context.WithCancel(context.Background())

	targets := []Target{
		{
			Name:          "fragile",
			Interval:      30 * time.Millisecond,
			FailThreshold: 3,
			Checker:       chk,
		},
	}

	sched.StartAsync(ctx, targets)
	time.Sleep(250 * time.Millisecond)
	cancel()

	events := n.Events()

	// Should have exactly 1 DOWN alert (threshold=3, first 3 are down)
	downAlerts := 0
	for _, e := range events {
		if e.NewStatus == checker.StatusDown {
			downAlerts++
		}
	}
	if downAlerts != 1 {
		t.Errorf("expected exactly 1 DOWN alert, got %d", downAlerts)
	}

	// Verify state
	states := sched.GetState()
	if states["fragile"] != checker.StatusDown {
		t.Errorf("expected state DOWN, got %v", states["fragile"])
	}
}

func TestSchedulerStateTransitionRecovery(t *testing.T) {
	db := newTestStore(t)
	n := &mockNotifier{}

	// 3 failures then recovery (add explicit ups after to avoid repeating last)
	chk := &mockChecker{
		results: []checker.Result{
			{Status: checker.StatusDown, Error: "fail 1", Latency: time.Millisecond},
			{Status: checker.StatusDown, Error: "fail 2", Latency: time.Millisecond},
			{Status: checker.StatusDown, Error: "fail 3", Latency: time.Millisecond},
			{Status: checker.StatusUp, Latency: 5 * time.Millisecond},
			{Status: checker.StatusUp, Latency: 5 * time.Millisecond},
			{Status: checker.StatusUp, Latency: 5 * time.Millisecond},
			{Status: checker.StatusUp, Latency: 5 * time.Millisecond},
			{Status: checker.StatusUp, Latency: 5 * time.Millisecond},
			{Status: checker.StatusUp, Latency: 5 * time.Millisecond},
			{Status: checker.StatusUp, Latency: 5 * time.Millisecond},
		},
	}

	sched := New(db, n)
	ctx, cancel := context.WithCancel(context.Background())

	targets := []Target{
		{
			Name:          "recoverable",
			Interval:      30 * time.Millisecond,
			FailThreshold: 3,
			Checker:       chk,
		},
	}

	sched.StartAsync(ctx, targets)
	time.Sleep(300 * time.Millisecond)
	cancel()

	events := n.Events()

	hasDown := false
	hasUp := false
	for _, e := range events {
		if e.NewStatus == checker.StatusDown {
			hasDown = true
		}
		if e.NewStatus == checker.StatusUp && e.PrevStatus == checker.StatusDown {
			hasUp = true
		}
	}

	if !hasDown {
		t.Error("expected DOWN alert")
	}
	if !hasUp {
		t.Error("expected recovery (UP) alert")
	}

	// Final state should be up
	states := sched.GetState()
	if states["recoverable"] != checker.StatusUp {
		t.Errorf("expected state UP after recovery, got %v", states["recoverable"])
	}
}

func TestSchedulerNoAlertBelowThreshold(t *testing.T) {
	db := newTestStore(t)
	n := &mockNotifier{}

	// Only 2 failures then recovery — should NOT trigger alert (threshold=3)
	chk := &mockChecker{
		results: []checker.Result{
			{Status: checker.StatusDown, Error: "fail 1", Latency: time.Millisecond},
			{Status: checker.StatusDown, Error: "fail 2", Latency: time.Millisecond},
			{Status: checker.StatusUp, Latency: time.Millisecond},
		},
	}

	sched := New(db, n)
	ctx, cancel := context.WithCancel(context.Background())

	targets := []Target{
		{
			Name:          "resilient",
			Interval:      30 * time.Millisecond,
			FailThreshold: 3,
			Checker:       chk,
		},
	}

	sched.StartAsync(ctx, targets)
	time.Sleep(200 * time.Millisecond)
	cancel()

	events := n.Events()
	for _, e := range events {
		if e.NewStatus == checker.StatusDown {
			t.Error("should not alert when below fail threshold")
		}
	}
}

func TestSchedulerGetStateEmpty(t *testing.T) {
	db := newTestStore(t)
	n := &mockNotifier{}
	sched := New(db, n)

	states := sched.GetState()
	if len(states) != 0 {
		t.Errorf("expected empty state map, got %d entries", len(states))
	}
}

func TestSchedulerStartBlocks(t *testing.T) {
	db := newTestStore(t)
	n := &mockNotifier{}
	chk := &mockChecker{}

	sched := New(db, n)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		sched.Start(ctx, []Target{
			{Name: "blocking", Interval: 50 * time.Millisecond, FailThreshold: 3, Checker: chk},
		})
		close(done)
	}()

	// Start should block until context is cancelled
	time.Sleep(100 * time.Millisecond)
	select {
	case <-done:
		t.Error("Start should block until context cancellation")
	default:
		// expected
	}

	cancel()
	select {
	case <-done:
		// expected
	case <-time.After(2 * time.Second):
		t.Error("Start should return after context cancellation")
	}
}

func TestSchedulerMultipleTargets(t *testing.T) {
	db := newTestStore(t)
	n := &mockNotifier{}

	chk1 := &mockChecker{}
	chk2 := &mockChecker{}

	sched := New(db, n)
	ctx, cancel := context.WithCancel(context.Background())

	targets := []Target{
		{Name: "ep1", Interval: 50 * time.Millisecond, FailThreshold: 3, Checker: chk1},
		{Name: "ep2", Interval: 50 * time.Millisecond, FailThreshold: 3, Checker: chk2},
	}

	sched.StartAsync(ctx, targets)
	time.Sleep(300 * time.Millisecond)
	cancel()

	if chk1.CallCount() < 2 {
		t.Errorf("ep1: expected at least 2 checks, got %d", chk1.CallCount())
	}
	if chk2.CallCount() < 2 {
		t.Errorf("ep2: expected at least 2 checks, got %d", chk2.CallCount())
	}
}
