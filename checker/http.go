package checker

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTP performs HTTP/HTTPS health checks.
type HTTP struct {
	url            string
	method         string
	expectedStatus int
	expectedBody   string
	headers        map[string]string
	timeout        time.Duration
	client         *http.Client
}

// NewHTTP creates an HTTP checker.
func NewHTTP(url, method string, expectedStatus int, expectedBody string, headers map[string]string, timeout time.Duration) *HTTP {
	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{},
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableKeepAlives:   false,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		// Don't follow redirects — we want to see the actual status code
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	if method == "" {
		method = "GET"
	}

	return &HTTP{
		url:            url,
		method:         method,
		expectedStatus: expectedStatus,
		expectedBody:   expectedBody,
		headers:        headers,
		timeout:        timeout,
		client:         client,
	}
}

// Check performs the HTTP health check.
func (h *HTTP) Check(ctx context.Context) Result {
	start := time.Now()
	result := Result{Timestamp: start}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, h.method, h.url, nil)
	if err != nil {
		result.Status = StatusDown
		result.Error = err.Error()
		result.Latency = time.Since(start)
		return result
	}

	req.Header.Set("User-Agent", "Pulse/1.0 (Uptime Monitor)")
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		result.Status = StatusDown
		result.Error = err.Error()
		result.Latency = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Latency = time.Since(start)

	// Check status code
	if h.expectedStatus > 0 && resp.StatusCode != h.expectedStatus {
		result.Status = StatusDown
		result.Error = "unexpected status code"
		return result
	} else if h.expectedStatus == 0 && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		result.Status = StatusDown
		result.Error = "non-2xx status code"
		return result
	}

	// Check body content if configured
	if h.expectedBody != "" {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
		if err != nil {
			result.Status = StatusDown
			result.Error = "failed to read response body"
			return result
		}
		if !strings.Contains(string(body), h.expectedBody) {
			result.Status = StatusDown
			result.Error = "expected body content not found"
			return result
		}
	}

	result.Status = StatusUp
	return result
}
