package health

import (
	"context"
	"fmt"
	"net"
	"time"
)

type SFTPCheck struct {
	label      string
	host       string
	port       int
	timeout    time.Duration
}

func NewSFTPCheck(host string, port int) *SFTPCheck {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 22
	}
	return &SFTPCheck{
		label:   "SFTP Connectivity",
		host:    host,
		port:    port,
		timeout: 5 * time.Second,
	}
}

func (c *SFTPCheck) Name() string  { return "sftp" }
func (c *SFTPCheck) Label() string { return c.label }
func (c *SFTPCheck) Critical() bool { return true }

func (c *SFTPCheck) Run(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:   c.Name(),
		Label:  c.Label(),
		Status: StatusOK,
	}

	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(c.host, fmt.Sprintf("%d", c.port)))
	if err != nil {
		result.Status = StatusFailed
		result.Message = "SFTP endpoint unreachable: " + err.Error()
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}
	conn.Close()

	result.Message = "SFTP endpoint reachable"
	result.LatencyMs = time.Since(start).Milliseconds()
	result.Details = map[string]any{
		"host":    c.host,
		"port":    c.port,
		"latency": result.LatencyMs,
	}

	return result
}
