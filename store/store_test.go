package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jack-rowe/pulse/checker"
)

func newTestStore(t *testing.T) *BoltStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := NewBolt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewBolt(t *testing.T) {
	s := newTestStore(t)
	if s == nil {
		t.Fatal("store should not be nil")
	}
}

func TestNewBoltInvalidPath(t *testing.T) {
	_, err := NewBolt("/nonexistent/dir/deep/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestSaveAndGetLatest(t *testing.T) {
	s := newTestStore(t)

	rec := CheckRecord{
		EndpointName: "api",
		Status:       checker.StatusUp,
		StatusCode:   200,
		LatencyMs:    42.5,
		Timestamp:    time.Now(),
	}

	if err := s.SaveResult(rec); err != nil {
		t.Fatal(err)
	}

	latest, err := s.GetLatest()
	if err != nil {
		t.Fatal(err)
	}

	if len(latest) != 1 {
		t.Fatalf("expected 1 latest record, got %d", len(latest))
	}
	if latest[0].EndpointName != "api" {
		t.Errorf("expected endpoint 'api', got %q", latest[0].EndpointName)
	}
	if latest[0].Status != checker.StatusUp {
		t.Errorf("expected StatusUp, got %v", latest[0].Status)
	}
	if latest[0].StatusCode != 200 {
		t.Errorf("expected status code 200, got %d", latest[0].StatusCode)
	}
}

func TestLatestUpdatesOnNewResult(t *testing.T) {
	s := newTestStore(t)

	// Save first result
	rec1 := CheckRecord{
		EndpointName: "api",
		Status:       checker.StatusUp,
		LatencyMs:    10,
		Timestamp:    time.Now().Add(-time.Minute),
	}
	s.SaveResult(rec1)

	// Save second result (newer)
	rec2 := CheckRecord{
		EndpointName: "api",
		Status:       checker.StatusDown,
		LatencyMs:    500,
		Error:        "timeout",
		Timestamp:    time.Now(),
	}
	s.SaveResult(rec2)

	latest, _ := s.GetLatest()
	if len(latest) != 1 {
		t.Fatalf("expected 1 latest, got %d", len(latest))
	}
	if latest[0].Status != checker.StatusDown {
		t.Error("latest should reflect most recent save")
	}
}

func TestGetHistory(t *testing.T) {
	s := newTestStore(t)
	base := time.Now()

	for i := 0; i < 10; i++ {
		s.SaveResult(CheckRecord{
			EndpointName: "api",
			Status:       checker.StatusUp,
			LatencyMs:    float64(i * 10),
			Timestamp:    base.Add(time.Duration(i) * time.Second),
		})
	}

	// Get all history
	history, err := s.GetHistory("api", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 10 {
		t.Errorf("expected 10 records, got %d", len(history))
	}

	// Should be newest first
	if history[0].Timestamp.Before(history[len(history)-1].Timestamp) {
		t.Error("history should be ordered newest first")
	}
}

func TestGetHistoryWithLimit(t *testing.T) {
	s := newTestStore(t)
	base := time.Now()

	for i := 0; i < 10; i++ {
		s.SaveResult(CheckRecord{
			EndpointName: "api",
			Status:       checker.StatusUp,
			Timestamp:    base.Add(time.Duration(i) * time.Second),
		})
	}

	history, _ := s.GetHistory("api", 3)
	if len(history) != 3 {
		t.Errorf("expected 3 records with limit, got %d", len(history))
	}
}

func TestGetHistoryNonexistentEndpoint(t *testing.T) {
	s := newTestStore(t)

	history, err := s.GetHistory("nonexistent", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Errorf("expected 0 records for nonexistent endpoint, got %d", len(history))
	}
}

func TestGetUptimeSummary(t *testing.T) {
	s := newTestStore(t)
	base := time.Now()

	// 8 up, 2 down
	for i := 0; i < 10; i++ {
		status := checker.StatusUp
		errMsg := ""
		if i >= 8 {
			status = checker.StatusDown
			errMsg = "fail"
		}
		s.SaveResult(CheckRecord{
			EndpointName: "api",
			Status:       status,
			LatencyMs:    float64((i + 1) * 10),
			Error:        errMsg,
			Timestamp:    base.Add(time.Duration(i) * time.Second),
		})
	}

	summary, err := s.GetUptimeSummary("api", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if summary.TotalChecks != 10 {
		t.Errorf("expected 10 checks, got %d", summary.TotalChecks)
	}
	if summary.TotalFailures != 2 {
		t.Errorf("expected 2 failures, got %d", summary.TotalFailures)
	}
	if summary.UptimePercent != 80 {
		t.Errorf("expected 80%% uptime, got %.1f%%", summary.UptimePercent)
	}
	if summary.MinLatencyMs != 10 {
		t.Errorf("expected min latency 10, got %.1f", summary.MinLatencyMs)
	}
	if summary.MaxLatencyMs != 100 {
		t.Errorf("expected max latency 100, got %.1f", summary.MaxLatencyMs)
	}
	if summary.AvgLatencyMs != 55 {
		t.Errorf("expected avg latency 55, got %.1f", summary.AvgLatencyMs)
	}
	if summary.LastDownAt == nil {
		t.Error("expected LastDownAt to be set")
	}
}

func TestGetUptimeSummaryNoData(t *testing.T) {
	s := newTestStore(t)

	summary, err := s.GetUptimeSummary("nonexistent", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalChecks != 0 {
		t.Errorf("expected 0 checks, got %d", summary.TotalChecks)
	}
}

func TestPurgeOlderThan(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	// 5 old records
	for i := 0; i < 5; i++ {
		s.SaveResult(CheckRecord{
			EndpointName: "api",
			Status:       checker.StatusUp,
			Timestamp:    now.Add(-48 * time.Hour).Add(time.Duration(i) * time.Second),
		})
	}
	// 5 recent records
	for i := 0; i < 5; i++ {
		s.SaveResult(CheckRecord{
			EndpointName: "api",
			Status:       checker.StatusUp,
			Timestamp:    now.Add(-time.Duration(i) * time.Second),
		})
	}

	cutoff := now.Add(-24 * time.Hour)
	deleted, err := s.PurgeOlderThan(cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 5 {
		t.Errorf("expected 5 deleted, got %d", deleted)
	}

	history, _ := s.GetHistory("api", 0)
	if len(history) != 5 {
		t.Errorf("expected 5 remaining, got %d", len(history))
	}
}

func TestPurgeNoData(t *testing.T) {
	s := newTestStore(t)

	deleted, err := s.PurgeOlderThan(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}

func TestGetTimeline(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	// Create records well within a 2-hour window to avoid boundary issues
	for i := 0; i < 60; i++ {
		status := checker.StatusUp
		if i >= 50 {
			status = checker.StatusDown
		}
		// Place records starting 110 minutes ago, 1 min apart, so all fit within 2h window
		s.SaveResult(CheckRecord{
			EndpointName: "api",
			Status:       status,
			LatencyMs:    float64(i + 1),
			Timestamp:    now.Add(-110 * time.Minute).Add(time.Duration(i) * time.Minute),
		})
	}

	buckets, err := s.GetTimeline("api", 2*time.Hour, 6)
	if err != nil {
		t.Fatal(err)
	}

	if len(buckets) != 6 {
		t.Fatalf("expected 6 buckets, got %d", len(buckets))
	}

	// Verify bucket structure
	for i, b := range buckets {
		if b.Start.After(b.End) {
			t.Errorf("bucket %d: start after end", i)
		}
		if i > 0 && !buckets[i-1].End.Equal(b.Start) {
			t.Errorf("bucket %d: not contiguous with previous", i)
		}
	}

	totalChecks := 0
	totalFails := 0
	for _, b := range buckets {
		totalChecks += b.TotalChecks
		totalFails += b.Failures
	}
	if totalChecks != 60 {
		t.Errorf("expected 60 total checks across buckets, got %d", totalChecks)
	}
	if totalFails != 10 {
		t.Errorf("expected 10 total failures, got %d", totalFails)
	}
}

func TestGetTimelineNoData(t *testing.T) {
	s := newTestStore(t)

	buckets, err := s.GetTimeline("nonexistent", time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 10 {
		t.Errorf("expected 10 empty buckets, got %d", len(buckets))
	}
	for _, b := range buckets {
		if b.Status != "empty" {
			t.Errorf("expected empty status, got %q", b.Status)
		}
	}
}

func TestMultipleEndpoints(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	endpoints := []string{"api", "db", "web"}
	for _, ep := range endpoints {
		for i := 0; i < 3; i++ {
			s.SaveResult(CheckRecord{
				EndpointName: ep,
				Status:       checker.StatusUp,
				LatencyMs:    float64(i),
				Timestamp:    now.Add(time.Duration(i) * time.Second),
			})
		}
	}

	latest, _ := s.GetLatest()
	if len(latest) != 3 {
		t.Errorf("expected 3 latest records (one per endpoint), got %d", len(latest))
	}

	for _, ep := range endpoints {
		history, _ := s.GetHistory(ep, 0)
		if len(history) != 3 {
			t.Errorf("endpoint %q: expected 3 history records, got %d", ep, len(history))
		}
	}
}

func TestTimeToKey(t *testing.T) {
	t1 := time.Now()
	t2 := t1.Add(time.Second)

	k1 := timeToKey(t1)
	k2 := timeToKey(t2)

	if len(k1) != 8 {
		t.Errorf("expected 8 byte key, got %d", len(k1))
	}

	// k2 should be greater (later time)
	for i := range k1 {
		if k1[i] < k2[i] {
			break // correct: k1 < k2
		}
		if k1[i] > k2[i] {
			t.Error("earlier time should produce smaller key")
			break
		}
	}
}
