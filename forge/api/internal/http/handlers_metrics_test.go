package http

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsCollector_RecordRequest(t *testing.T) {
	mc := NewMetricsCollector()
	mc.RecordRequest("GET", "/api/v1/servers", 200, 50*time.Millisecond)
	mc.RecordRequest("GET", "/api/v1/servers", 200, 30*time.Millisecond)
	mc.RecordRequest("POST", "/api/v1/servers", 201, 100*time.Millisecond)

	output := mc.FormatPrometheus()
	if !strings.Contains(output, "http_requests_total") {
		t.Fatal("expected http_requests_total in output")
	}
	if !strings.Contains(output, "http_request_duration_seconds") {
		t.Fatal("expected http_request_duration_seconds in output")
	}
}

func TestMetricsCollector_RecordError(t *testing.T) {
	mc := NewMetricsCollector()
	mc.RecordError("timeout")
	mc.RecordError("timeout")
	mc.RecordError("internal")

	output := mc.FormatPrometheus()
	if !strings.Contains(output, "http_errors_total") {
		t.Fatal("expected http_errors_total in output")
	}
	if !strings.Contains(output, `type="timeout"`) {
		t.Fatal("expected timeout error type in output")
	}
	if !strings.Contains(output, `type="internal"`) {
		t.Fatal("expected internal error type in output")
	}
}

func TestMetricsCollector_ActiveConnections(t *testing.T) {
	mc := NewMetricsCollector()
	mc.IncrementActiveConnections()
	mc.IncrementActiveConnections()
	mc.IncrementActiveConnections()
	mc.DecrementActiveConnections()

	output := mc.FormatPrometheus()
	if !strings.Contains(output, "http_active_connections 2") {
		t.Fatalf("expected 2 active connections in output, got:\n%s", output)
	}
}

func TestMetricsCollector_ActiveConnections_NoNegative(t *testing.T) {
	mc := NewMetricsCollector()
	mc.DecrementActiveConnections()
	mc.DecrementActiveConnections()

	output := mc.FormatPrometheus()
	if !strings.Contains(output, "http_active_connections 0") {
		t.Fatalf("expected 0 active connections, got:\n%s", output)
	}
}

func TestMetricsCollector_FormatPrometheus(t *testing.T) {
	mc := NewMetricsCollector()
	output := mc.FormatPrometheus()

	if !strings.Contains(output, "process_uptime_seconds") {
		t.Fatal("expected process_uptime_seconds in output")
	}
	if !strings.Contains(output, "# HELP") {
		t.Fatal("expected HELP comments in output")
	}
	if !strings.Contains(output, "# TYPE") {
		t.Fatal("expected TYPE comments in output")
	}
}

func TestMetricsHandlers_HandleMetrics(t *testing.T) {
	mc := NewMetricsCollector()
	mc.RecordRequest("GET", "/test", 200, 10*time.Millisecond)
	h := NewMetricsHandlers(mc)

	app := bareTestApp()
	app.Get("/metrics", h.HandleMetrics)

	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("expected text/plain content type, got %s", ct)
	}
}
