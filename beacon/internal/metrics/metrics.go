package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsCollector defines an interface for collecting metrics
type MetricsCollector interface {
	RecordServerStatus(status string)
	RecordBackupDuration(duration time.Duration)
	RecordRequestLatency(method, path string, duration time.Duration)
}

// PrometheusCollector implements MetricsCollector using Prometheus
type PrometheusCollector struct {
	serverStatus   *prometheus.GaugeVec
	backupDuration prometheus.Histogram
	requestLatency *prometheus.HistogramVec
}

// NewPrometheusCollector creates a new PrometheusCollector
func NewPrometheusCollector() *PrometheusCollector {
	serverStatus := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "server_status",
			Help: "Current status of the server",
		},
		[]string{"status"},
	)

	backupDuration := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backup_duration_seconds",
			Help:    "Duration of backup operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	requestLatency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "request_latency_seconds",
			Help:    "Request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	prometheus.MustRegister(serverStatus, backupDuration, requestLatency)

	return &PrometheusCollector{
		serverStatus:   serverStatus,
		backupDuration: backupDuration,
		requestLatency: requestLatency,
	}
}

// RecordServerStatus records the current server status
func (p *PrometheusCollector) RecordServerStatus(status string) {
	p.serverStatus.WithLabelValues(status).Set(1)
}

// RecordBackupDuration records the duration of a backup operation
func (p *PrometheusCollector) RecordBackupDuration(duration time.Duration) {
	p.backupDuration.Observe(duration.Seconds())
}

// RecordRequestLatency records the latency of a request
func (p *PrometheusCollector) RecordRequestLatency(method, path string, duration time.Duration) {
	p.requestLatency.WithLabelValues(method, path).Observe(duration.Seconds())
}
