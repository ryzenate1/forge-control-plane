package http

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

type MetricsCollector struct {
	mu                sync.RWMutex
	requestCounts     map[string]int64
	requestDurations  map[string][]time.Duration
	activeConnections int64
	errorCounts       map[string]int64
	startTime         time.Time
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		requestCounts:    make(map[string]int64),
		requestDurations: make(map[string][]time.Duration),
		errorCounts:      make(map[string]int64),
		startTime:        time.Now(),
	}
}

func (m *MetricsCollector) RecordRequest(method, path string, status int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s_%s_%d", method, path, status)
	m.requestCounts[key]++
	m.requestDurations[key] = append(m.requestDurations[key], duration)
	if len(m.requestDurations[key]) > 1000 {
		m.requestDurations[key] = m.requestDurations[key][len(m.requestDurations[key])-1000:]
	}
}

func (m *MetricsCollector) RecordError(errType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorCounts[errType]++
}

func (m *MetricsCollector) IncrementActiveConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeConnections++
}

func (m *MetricsCollector) DecrementActiveConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeConnections--
	if m.activeConnections < 0 {
		m.activeConnections = 0
	}
}

func (m *MetricsCollector) FormatPrometheus() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var b strings.Builder

	b.WriteString("# HELP http_requests_total Total number of HTTP requests.\n")
	b.WriteString("# TYPE http_requests_total counter\n")
	keys := make([]string, 0, len(m.requestCounts))
	for k := range m.requestCounts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts := strings.SplitN(key, "_", 3)
		if len(parts) == 3 {
			b.WriteString(fmt.Sprintf("http_requests_total{method=\"%s\",path=\"%s\",status=\"%s\"} %d\n",
				parts[0], parts[1], parts[2], m.requestCounts[key]))
		}
	}

	b.WriteString("# HELP http_request_duration_seconds HTTP request duration in seconds.\n")
	b.WriteString("# TYPE http_request_duration_seconds histogram\n")
	for _, key := range keys {
		durations := m.requestDurations[key]
		if len(durations) == 0 {
			continue
		}
		var sum float64
		for _, d := range durations {
			sum += d.Seconds()
		}
		parts := strings.SplitN(key, "_", 3)
		if len(parts) == 3 {
			b.WriteString(fmt.Sprintf("http_request_duration_seconds_sum{method=\"%s\",path=\"%s\",status=\"%s\"} %.6f\n",
				parts[0], parts[1], parts[2], sum))
			b.WriteString(fmt.Sprintf("http_request_duration_seconds_count{method=\"%s\",path=\"%s\",status=\"%s\"} %d\n",
				parts[0], parts[1], parts[2], len(durations)))
		}
	}

	b.WriteString("# HELP http_active_connections Current number of active connections.\n")
	b.WriteString("# TYPE http_active_connections gauge\n")
	b.WriteString(fmt.Sprintf("http_active_connections %d\n", m.activeConnections))

	b.WriteString("# HELP http_errors_total Total number of HTTP errors by type.\n")
	b.WriteString("# TYPE http_errors_total counter\n")
	errKeys := make([]string, 0, len(m.errorCounts))
	for k := range m.errorCounts {
		errKeys = append(errKeys, k)
	}
	sort.Strings(errKeys)
	for _, key := range errKeys {
		b.WriteString(fmt.Sprintf("http_errors_total{type=\"%s\"} %d\n", key, m.errorCounts[key]))
	}

	b.WriteString("# HELP process_uptime_seconds Process uptime in seconds.\n")
	b.WriteString("# TYPE process_uptime_seconds gauge\n")
	b.WriteString(fmt.Sprintf("process_uptime_seconds %.3f\n", time.Since(m.startTime).Seconds()))

	return b.String()
}

type MetricsHandlers struct {
	collector *MetricsCollector
}

func NewMetricsHandlers(collector *MetricsCollector) *MetricsHandlers {
	return &MetricsHandlers{collector: collector}
}

func (h *MetricsHandlers) HandleMetrics(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	return c.SendString(h.collector.FormatPrometheus())
}
