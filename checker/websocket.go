package checker

import (
	"context"
	"fmt"
	"time"

	"nhooyr.io/websocket"
)

// WebSocket performs WebSocket handshake health checks.
type WebSocket struct {
	url     string
	timeout time.Duration
}

// NewWebSocket creates a WebSocket checker.
func NewWebSocket(url string, timeout time.Duration) *WebSocket {
	return &WebSocket{
		url:     url,
		timeout: timeout,
	}
}

// Check performs a WebSocket handshake and immediately closes.
func (w *WebSocket) Check(ctx context.Context) Result {
	start := time.Now()
	result := Result{Timestamp: start}

	ctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, w.url, nil)
	result.Latency = time.Since(start)

	if err != nil {
		result.Status = StatusDown
		result.Error = fmt.Sprintf("websocket dial failed: %v", err)
		return result
	}
	conn.Close(websocket.StatusNormalClosure, "pulse health check")

	result.Status = StatusUp
	return result
}
