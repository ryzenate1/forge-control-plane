package runtime

import (
	"strings"
	"testing"
)

func TestDecodeEnhancedStats(t *testing.T) {
	stats, err := DecodeEnhancedStats(strings.NewReader(`{
		"cpu_stats": {
			"cpu_usage": {"total_usage": 200, "percpu_usage": [100, 100]},
			"system_cpu_usage": 1000,
			"online_cpus": 2
		},
		"precpu_stats": {
			"cpu_usage": {"total_usage": 100},
			"system_cpu_usage": 500
		},
		"memory_stats": {"usage": 524288, "limit": 1048576},
		"networks": {
			"eth0": {"rx_bytes": 1048576, "tx_bytes": 2097152}
		},
		"blkio_stats": {
			"io_service_bytes_recursive": [
				{"op": "Read", "value": 4096},
				{"op": "Write", "value": 8192}
			]
		},
		"pids_stats": {"current": 5}
	}`))
	if err != nil {
		t.Fatal(err)
	}

	if stats.CPUPercent != 40 {
		t.Fatalf("expected CPU 40, got %f", stats.CPUPercent)
	}
	if stats.CPUCount != 2 {
		t.Fatalf("expected CPUCount 2, got %d", stats.CPUCount)
	}
	if stats.MemoryUsage != 524288 {
		t.Fatalf("expected MemoryUsage 524288, got %d", stats.MemoryUsage)
	}
	if stats.MemoryLimit != 1048576 {
		t.Fatalf("expected MemoryLimit 1048576, got %d", stats.MemoryLimit)
	}
	if stats.MemoryPercent != 50 {
		t.Fatalf("expected MemoryPercent 50, got %f", stats.MemoryPercent)
	}
	if stats.NetworkRx != 1048576 {
		t.Fatalf("expected NetworkRx 1048576, got %d", stats.NetworkRx)
	}
	if stats.NetworkTx != 2097152 {
		t.Fatalf("expected NetworkTx 2097152, got %d", stats.NetworkTx)
	}
	if stats.BlockRead != 4096 {
		t.Fatalf("expected BlockRead 4096, got %d", stats.BlockRead)
	}
	if stats.BlockWrite != 8192 {
		t.Fatalf("expected BlockWrite 8192, got %d", stats.BlockWrite)
	}
	if stats.PIDs != 5 {
		t.Fatalf("expected PIDs 5, got %d", stats.PIDs)
	}
	if stats.Timestamp.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestDecodeEnhancedStatsCPUFallback(t *testing.T) {
	stats, err := DecodeEnhancedStats(strings.NewReader(`{
		"cpu_stats": {
			"cpu_usage": {"total_usage": 200, "percpu_usage": [50, 50, 50, 50]},
			"system_cpu_usage": 1000
		},
		"precpu_stats": {
			"cpu_usage": {"total_usage": 100},
			"system_cpu_usage": 500
		},
		"memory_stats": {"usage": 0, "limit": 0},
		"networks": {},
		"blkio_stats": {"io_service_bytes_recursive": null},
		"pids_stats": {"current": 0}
	}`))
	if err != nil {
		t.Fatal(err)
	}

	if stats.CPUCount != 4 {
		t.Fatalf("expected CPUCount 4 from percpu_usage fallback, got %d", stats.CPUCount)
	}
	if stats.MemoryPercent != 0 {
		t.Fatalf("expected MemoryPercent 0 with zero limit, got %f", stats.MemoryPercent)
	}
	if stats.BlockRead != 0 || stats.BlockWrite != 0 {
		t.Fatalf("expected zero block I/O, got read=%d write=%d", stats.BlockRead, stats.BlockWrite)
	}
}

func TestDecodeEnhancedStatsZeroDelta(t *testing.T) {
	stats, err := DecodeEnhancedStats(strings.NewReader(`{
		"cpu_stats": {
			"cpu_usage": {"total_usage": 100, "percpu_usage": [100]},
			"system_cpu_usage": 500,
			"online_cpus": 1
		},
		"precpu_stats": {
			"cpu_usage": {"total_usage": 100},
			"system_cpu_usage": 500
		},
		"memory_stats": {"usage": 100, "limit": 200},
		"networks": {},
		"blkio_stats": {},
		"pids_stats": {}
	}`))
	if err != nil {
		t.Fatal(err)
	}

	if stats.CPUPercent != 0 {
		t.Fatalf("expected CPU 0 with zero delta, got %f", stats.CPUPercent)
	}
}

func TestDecodeEnhancedStatsInvalidJSON(t *testing.T) {
	_, err := DecodeEnhancedStats(strings.NewReader(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
