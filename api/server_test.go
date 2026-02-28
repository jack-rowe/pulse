package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jack-rowe/pulse/checker"
	"github.com/jack-rowe/pulse/config"
	"github.com/jack-rowe/pulse/store"
)

func newTestStore(t *testing.T) *store.BoltStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := store.NewBolt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedStore(t *testing.T, s *store.BoltStore) {
	t.Helper()
	now := time.Now()
	for i := 0; i < 5; i++ {
		s.SaveResult(store.CheckRecord{
			EndpointName: "API Server",
			Status:       checker.StatusUp,
			StatusCode:   200,
			LatencyMs:    float64(20 + i),
			Timestamp:    now.Add(-time.Duration(i) * time.Minute),
		})
	}
	// One failure
	s.SaveResult(store.CheckRecord{
		EndpointName: "API Server",
		Status:       checker.StatusDown,
		StatusCode:   503,
		LatencyMs:    500,
		Error:        "service unavailable",
		Timestamp:    now.Add(-10 * time.Minute),
	})
}

var testEndpoints = []config.Endpoint{
	{
		Name:           "API Server",
		Type:           "http",
		URL:            "https://api.example.com/health",
		ExpectedStatus: 200,
		TimeoutMs:      5000,
		IntervalSec:    30,
	},
}

func TestHealthEndpoint(t *testing.T) {
	db := newTestStore(t)
	srv := NewServer(db, testEndpoints, "")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
}

func TestStatusEndpoint(t *testing.T) {
	db := newTestStore(t)
	seedStore(t, db)
	srv := NewServer(db, testEndpoints, "")

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp StatusResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(resp.Endpoints))
	}

	ep := resp.Endpoints[0]
	if ep.Name != "API Server" {
		t.Errorf("expected name 'API Server', got %q", ep.Name)
	}
	if ep.Status == "unknown" {
		t.Error("status should not be unknown with seeded data")
	}
	if ep.TotalChecks == 0 {
		t.Error("expected non-zero total checks")
	}
}

func TestStatusEndpointDetail(t *testing.T) {
	db := newTestStore(t)
	seedStore(t, db)
	srv := NewServer(db, testEndpoints, "")

	req := httptest.NewRequest("GET", "/api/status/API%20Server", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var summary store.UptimeSummary
	json.NewDecoder(w.Body).Decode(&summary)
	if summary.TotalChecks == 0 {
		t.Error("expected non-zero total checks")
	}
}

func TestStatusEndpointNotFound(t *testing.T) {
	db := newTestStore(t)
	srv := NewServer(db, testEndpoints, "")

	req := httptest.NewRequest("GET", "/api/status/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHistoryEndpoint(t *testing.T) {
	db := newTestStore(t)
	seedStore(t, db)
	srv := NewServer(db, testEndpoints, "")

	req := httptest.NewRequest("GET", "/api/history/API%20Server", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["endpoint"] != "API Server" {
		t.Errorf("expected endpoint 'API Server', got %v", resp["endpoint"])
	}
	count := int(resp["count"].(float64))
	if count == 0 {
		t.Error("expected non-zero record count")
	}
}

func TestHistoryEndpointWithLimit(t *testing.T) {
	db := newTestStore(t)
	seedStore(t, db)
	srv := NewServer(db, testEndpoints, "")

	req := httptest.NewRequest("GET", "/api/history/API%20Server?limit=2", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	count := int(resp["count"].(float64))
	if count != 2 {
		t.Errorf("expected 2 records with limit, got %d", count)
	}
}

func TestTimelineEndpoint(t *testing.T) {
	db := newTestStore(t)
	seedStore(t, db)
	srv := NewServer(db, testEndpoints, "")

	req := httptest.NewRequest("GET", "/api/timeline/API%20Server?hours=1&buckets=6", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	buckets := resp["buckets"].([]any)
	if len(buckets) != 6 {
		t.Errorf("expected 6 buckets, got %d", len(buckets))
	}
}

func TestStatusPageEndpoint(t *testing.T) {
	db := newTestStore(t)
	srv := NewServer(db, testEndpoints, "")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %q", ct)
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("expected X-Frame-Options: DENY")
	}
	if w.Body.Len() == 0 {
		t.Error("expected non-empty HTML body")
	}
}

// --- Auth middleware tests ---

func TestAuthNoKeyConfigured(t *testing.T) {
	db := newTestStore(t)
	srv := NewServer(db, testEndpoints, "") // no API key

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with no key configured, got %d", w.Code)
	}
}

func TestAuthValidKey(t *testing.T) {
	db := newTestStore(t)
	srv := NewServer(db, testEndpoints, "my-secret")

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("X-API-Key", "my-secret")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid key, got %d", w.Code)
	}
}

func TestAuthInvalidKey(t *testing.T) {
	db := newTestStore(t)
	srv := NewServer(db, testEndpoints, "my-secret")

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong key, got %d", w.Code)
	}
}

func TestAuthMissingKey(t *testing.T) {
	db := newTestStore(t)
	srv := NewServer(db, testEndpoints, "my-secret")

	req := httptest.NewRequest("GET", "/api/status", nil)
	// no X-API-Key header
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with missing key, got %d", w.Code)
	}
}

func TestAuthHealthBypassesKey(t *testing.T) {
	db := newTestStore(t)
	srv := NewServer(db, testEndpoints, "my-secret")

	// /health should not require auth
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /health without key, got %d", w.Code)
	}
}

func TestResponseContentType(t *testing.T) {
	db := newTestStore(t)
	srv := NewServer(db, testEndpoints, "")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}
