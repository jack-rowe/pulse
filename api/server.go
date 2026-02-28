// Package api provides the HTTP status API and embedded status page.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jack-rowe/pulse/config"
	"github.com/jack-rowe/pulse/store"
)

// Server is the HTTP API for Pulse.
type Server struct {
	store     store.Store
	endpoints []config.Endpoint
	apiKey    string
	mux       *http.ServeMux
}

// NewServer creates an API server.
func NewServer(st store.Store, endpoints []config.Endpoint, apiKey string) *Server {
	s := &Server{
		store:     st,
		endpoints: endpoints,
		apiKey:    apiKey,
		mux:       http.NewServeMux(),
	}
	s.routes()
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /api/status", s.auth(s.handleStatus))
	s.mux.HandleFunc("GET /api/status/{name}", s.auth(s.handleEndpointStatus))
	s.mux.HandleFunc("GET /api/history/{name}", s.auth(s.handleHistory))
	s.mux.HandleFunc("GET /api/timeline/{name}", s.auth(s.handleTimeline))
	s.mux.HandleFunc("GET /", s.handleStatusPage)
}

// auth is middleware that checks the API key if configured.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey != "" {
			key := r.Header.Get("X-API-Key")
			if subtle.ConstantTimeCompare([]byte(key), []byte(s.apiKey)) != 1 {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

// GET /health — self health check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// StatusResponse is the JSON response for /api/status.
type StatusResponse struct {
	Endpoints []EndpointStatus `json:"endpoints"`
	Timestamp time.Time        `json:"timestamp"`
}

// EndpointStatus is the status of a single endpoint.
type EndpointStatus struct {
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	StatusCode    int     `json:"status_code,omitempty"`
	LatencyMs     float64 `json:"latency_ms"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	MinLatencyMs  float64 `json:"min_latency_ms"`
	MaxLatencyMs  float64 `json:"max_latency_ms"`
	UptimePercent float64 `json:"uptime_percent"`
	Uptime7d      float64 `json:"uptime_7d"`
	Uptime30d     float64 `json:"uptime_30d"`
	TotalChecks   int     `json:"total_checks"`
	TotalFailures int     `json:"total_failures"`
	LastChecked   string  `json:"last_checked"`
	LastDownAt    string  `json:"last_down_at,omitempty"`
	Error         string  `json:"error,omitempty"`
}

// GET /api/status — current state of all endpoints
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	latest, err := s.store.GetLatest()
	if err != nil {
		slog.Error("failed to get latest", "error", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	// Build lookup
	latestMap := make(map[string]store.CheckRecord)
	for _, rec := range latest {
		latestMap[rec.EndpointName] = rec
	}

	var statuses []EndpointStatus
	for _, ep := range s.endpoints {
		es := EndpointStatus{
			Name:   ep.Name,
			Status: "unknown",
		}

		if rec, ok := latestMap[ep.Name]; ok {
			es.Status = rec.Status.String()
			es.StatusCode = rec.StatusCode
			es.LatencyMs = rec.LatencyMs
			es.Error = rec.Error
			es.LastChecked = rec.Timestamp.UTC().Format(time.RFC3339)
		}

		// Get uptime percentage (last 24h)
		summary24h, err := s.store.GetUptimeSummary(ep.Name, 24*time.Hour)
		if err == nil && summary24h.TotalChecks > 0 {
			es.UptimePercent = summary24h.UptimePercent
			es.AvgLatencyMs = summary24h.AvgLatencyMs
			es.MinLatencyMs = summary24h.MinLatencyMs
			es.MaxLatencyMs = summary24h.MaxLatencyMs
			es.TotalChecks = summary24h.TotalChecks
			es.TotalFailures = summary24h.TotalFailures
			if summary24h.LastDownAt != nil {
				es.LastDownAt = summary24h.LastDownAt.UTC().Format(time.RFC3339)
			}
		}

		// 7-day uptime
		summary7d, err := s.store.GetUptimeSummary(ep.Name, 7*24*time.Hour)
		if err == nil && summary7d.TotalChecks > 0 {
			es.Uptime7d = summary7d.UptimePercent
		}

		// 30-day uptime
		summary30d, err := s.store.GetUptimeSummary(ep.Name, 30*24*time.Hour)
		if err == nil && summary30d.TotalChecks > 0 {
			es.Uptime30d = summary30d.UptimePercent
		}

		statuses = append(statuses, es)
	}

	writeJSON(w, http.StatusOK, StatusResponse{
		Endpoints: statuses,
		Timestamp: time.Now().UTC(),
	})
}

// GET /api/status/{name} — single endpoint detail
func (s *Server) handleEndpointStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	summary, err := s.store.GetUptimeSummary(name, 24*time.Hour)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	if summary.TotalChecks == 0 {
		http.Error(w, `{"error":"endpoint not found"}`, http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// GET /api/history/{name}?limit=50 — recent check history
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	limit := 50 // default

	history, err := s.store.GetHistory(name, limit)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint": name,
		"records":  history,
		"count":    len(history),
	})
}

// GET /api/timeline/{name}?hours=24&buckets=90 — bucketed timeline data
func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	hours := 24
	numBuckets := 90

	// Allow query params to override
	if h := r.URL.Query().Get("hours"); h != "" {
		if v, err := parsePositiveInt(h); err == nil {
			hours = v
		}
	}
	if b := r.URL.Query().Get("buckets"); b != "" {
		if v, err := parsePositiveInt(b); err == nil && v <= 200 {
			numBuckets = v
		}
	}

	duration := time.Duration(hours) * time.Hour
	timeline, err := s.store.GetTimeline(name, duration, numBuckets)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint": name,
		"hours":    hours,
		"buckets":  timeline,
	})
}

// GET / — embedded status page (HTML)
func (s *Server) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'")
	w.Write([]byte(statusPageHTML))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func parsePositiveInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return n, nil
}
