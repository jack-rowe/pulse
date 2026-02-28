package checker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Status tests ---

func TestStatusString(t *testing.T) {
	tests := []struct {
		s    Status
		want string
	}{
		{StatusUp, "up"},
		{StatusDown, "down"},
		{Status(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("Status(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

// --- HTTP checker tests ---

func TestHTTPCheckSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "healthy")
	}))
	defer srv.Close()

	chk := NewHTTP(srv.URL, "GET", 200, "", nil, 5*time.Second)
	result := chk.Check(context.Background())

	if result.Status != StatusUp {
		t.Errorf("expected StatusUp, got %v (error: %s)", result.Status, result.Error)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected status code 200, got %d", result.StatusCode)
	}
	if result.Latency <= 0 {
		t.Error("expected positive latency")
	}
}

func TestHTTPCheckExpectedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer srv.Close()

	chk := NewHTTP(srv.URL, "GET", 200, "ok", nil, 5*time.Second)
	result := chk.Check(context.Background())

	if result.Status != StatusUp {
		t.Errorf("expected StatusUp, got %v (error: %s)", result.Status, result.Error)
	}
}

func TestHTTPCheckBodyNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"error"}`)
	}))
	defer srv.Close()

	chk := NewHTTP(srv.URL, "GET", 200, "healthy", nil, 5*time.Second)
	result := chk.Check(context.Background())

	if result.Status != StatusDown {
		t.Errorf("expected StatusDown, got %v", result.Status)
	}
	if result.Error != "expected body content not found" {
		t.Errorf("unexpected error message: %s", result.Error)
	}
}

func TestHTTPCheckWrongStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	chk := NewHTTP(srv.URL, "GET", 200, "", nil, 5*time.Second)
	result := chk.Check(context.Background())

	if result.Status != StatusDown {
		t.Errorf("expected StatusDown, got %v", result.Status)
	}
	if result.StatusCode != 503 {
		t.Errorf("expected status code 503, got %d", result.StatusCode)
	}
}

func TestHTTPCheckNon2xxDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	// expectedStatus=0 means "any 2xx"
	chk := NewHTTP(srv.URL, "GET", 0, "", nil, 5*time.Second)
	result := chk.Check(context.Background())

	if result.Status != StatusDown {
		t.Errorf("expected StatusDown for 500 with no expected status, got %v", result.Status)
	}
}

func TestHTTPCheckConnectionRefused(t *testing.T) {
	chk := NewHTTP("http://127.0.0.1:1", "GET", 200, "", nil, 2*time.Second)
	result := chk.Check(context.Background())

	if result.Status != StatusDown {
		t.Errorf("expected StatusDown, got %v", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error")
	}
}

func TestHTTPCheckCustomHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "value" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	headers := map[string]string{"X-Custom": "value"}
	chk := NewHTTP(srv.URL, "GET", 200, "", headers, 5*time.Second)
	result := chk.Check(context.Background())

	if result.Status != StatusUp {
		t.Errorf("expected StatusUp with custom header, got %v (error: %s)", result.Status, result.Error)
	}
}

func TestHTTPCheckUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if !strings.Contains(ua, "Pulse") {
			t.Errorf("expected Pulse user agent, got %q", ua)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	chk := NewHTTP(srv.URL, "GET", 200, "", nil, 5*time.Second)
	chk.Check(context.Background())
}

func TestHTTPCheckContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	chk := NewHTTP(srv.URL, "GET", 200, "", nil, 5*time.Second)
	result := chk.Check(ctx)

	if result.Status != StatusDown {
		t.Errorf("expected StatusDown for cancelled context, got %v", result.Status)
	}
}

func TestHTTPCheckDefaultMethod(t *testing.T) {
	chk := NewHTTP("http://example.com", "", 200, "", nil, 5*time.Second)
	if chk.method != "GET" {
		t.Errorf("expected default method GET, got %q", chk.method)
	}
}

func TestHTTPCheckNoRedirectFollow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/other", http.StatusFound)
	}))
	defer srv.Close()

	chk := NewHTTP(srv.URL, "GET", 302, "", nil, 5*time.Second)
	result := chk.Check(context.Background())

	if result.Status != StatusUp {
		t.Errorf("expected StatusUp for redirect with expected 302, got %v (error: %s)", result.Status, result.Error)
	}
	if result.StatusCode != 302 {
		t.Errorf("expected status code 302, got %d", result.StatusCode)
	}
}

// --- TCP checker tests ---

func TestTCPCheckSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Accept connections in background
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	chk := NewTCP(ln.Addr().String(), 5*time.Second)
	result := chk.Check(context.Background())

	if result.Status != StatusUp {
		t.Errorf("expected StatusUp, got %v (error: %s)", result.Status, result.Error)
	}
	// Loopback can be sub-nanosecond; just verify no error
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
}

func TestTCPCheckConnectionRefused(t *testing.T) {
	chk := NewTCP("127.0.0.1:1", 2*time.Second)
	result := chk.Check(context.Background())

	if result.Status != StatusDown {
		t.Errorf("expected StatusDown, got %v", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error")
	}
}

func TestTCPCheckContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Use an address that won't immediately refuse (routable but unresponsive)
	chk := NewTCP("192.0.2.1:80", 10*time.Second)
	result := chk.Check(ctx)

	if result.Status != StatusDown {
		t.Errorf("expected StatusDown for cancelled context, got %v", result.Status)
	}
}

// --- Result tests ---

func TestResultTimestamp(t *testing.T) {
	before := time.Now()
	result := Result{Timestamp: time.Now()}
	if result.Timestamp.Before(before) {
		t.Error("timestamp should be at or after test start")
	}
}
