package checker

import (
	"context"
	"fmt"
	"net"
	"time"
)

// TCP performs raw TCP connectivity checks (port open/closed).
type TCP struct {
	address string
	timeout time.Duration
}

// NewTCP creates a TCP checker for the given host:port address.
func NewTCP(address string, timeout time.Duration) *TCP {
	return &TCP{
		address: address,
		timeout: timeout,
	}
}

// Check attempts a TCP connection to the target address.
func (t *TCP) Check(ctx context.Context) Result {
	start := time.Now()
	result := Result{Timestamp: start}

	dialer := &net.Dialer{Timeout: t.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", t.address)
	result.Latency = time.Since(start)

	if err != nil {
		result.Status = StatusDown
		result.Error = fmt.Sprintf("tcp dial failed: %v", err)
		return result
	}
	conn.Close()

	result.Status = StatusUp
	return result
}
