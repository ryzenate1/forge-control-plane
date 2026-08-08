package metrics

import (
	"regexp"
	stdruntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var registerOnce sync.Once
var dynamicPathSegment = regexp.MustCompile(`^(?:[0-9]+|[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12})$`)

// MetricsCollector defines an interface for collecting metrics
type MetricsCollector interface {
	RecordServerStatus(status string)
	RecordBackupDuration(duration time.Duration)
	RecordRequestLatency(method, path string, duration time.Duration)
}

// PrometheusCollector implements MetricsCollector using Prometheus
type PrometheusCollector struct {
	mu             sync.Mutex
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

	registerOnce.Do(func() {
		prometheus.MustRegister(serverStatus, backupDuration, requestLatency)
	})

	return &PrometheusCollector{
		serverStatus:   serverStatus,
		backupDuration: backupDuration,
		requestLatency: requestLatency,
	}
}

// RecordServerStatus records the current server status
func (p *PrometheusCollector) RecordServerStatus(status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.serverStatus.Reset()
	p.serverStatus.WithLabelValues(status).Set(1)
}

// RecordBackupDuration records the duration of a backup operation
func (p *PrometheusCollector) RecordBackupDuration(duration time.Duration) {
	p.backupDuration.Observe(duration.Seconds())
}

// RecordRequestLatency records the latency of a request
func (p *PrometheusCollector) RecordRequestLatency(method, path string, duration time.Duration) {
	p.requestLatency.WithLabelValues(strings.ToUpper(method), normalizeMetricPath(path)).Observe(duration.Seconds())
}

func normalizeMetricPath(value string) string {
	segments := strings.Split(strings.SplitN(value, "?", 2)[0], "/")
	for index, segment := range segments {
		if dynamicPathSegment.MatchString(segment) {
			segments[index] = ":id"
		}
	}
	value = strings.Join(segments, "/")
	if len(value) > 160 {
		return "/other"
	}
	return value
}

// ProcessMetrics captures process-level resource usage for the beacon daemon.
// The fields are consumed by the /metrics endpoint and rendered as Prometheus
// gauges and counters.
type ProcessMetrics struct {
	// StartTime is when the daemon process started; uptime is derived from it.
	StartTime time.Time
	// UserCPUSeconds and SystemCPUSeconds are cumulative process CPU times
	// reported by the operating system (getrusage).
	UserCPUSeconds   float64
	SystemCPUSeconds float64
	// MemAllocBytes is the currently allocated Go heap memory.
	MemAllocBytes uint64
	// MemHeapBytes is the heap bytes reserved by the Go runtime.
	MemHeapBytes uint64
	// Goroutines is the current number of goroutines.
	Goroutines int
	// NumGC is the number of completed garbage collection cycles.
	NumGC uint64
}

// CollectProcess samples the current process state. It is safe to call from
// the /metrics handler on every scrape.
func CollectProcess(started time.Time) ProcessMetrics {
	var mem stdruntime.MemStats
	stdruntime.ReadMemStats(&mem)
	userSeconds, systemSeconds := processCPUTimes()
	return ProcessMetrics{
		StartTime:        started,
		UserCPUSeconds:   userSeconds,
		SystemCPUSeconds: systemSeconds,
		MemAllocBytes:    mem.Alloc,
		MemHeapBytes:     mem.HeapSys,
		Goroutines:       stdruntime.NumGoroutine(),
		NumGC:            uint64(mem.NumGC),
	}
}
