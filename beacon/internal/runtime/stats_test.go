package runtime

import (
	"strings"
	"testing"
)

func TestDecodeDockerStats(t *testing.T) {
	stats, err := DecodeDockerStats(strings.NewReader(`{
		"cpu_stats": {
			"cpu_usage": {"total_usage": 200, "percpu_usage": [100, 100]},
			"system_cpu_usage": 1000
		},
		"precpu_stats": {
			"cpu_usage": {"total_usage": 100},
			"system_cpu_usage": 500
		},
		"memory_stats": {"usage": 512, "limit": 1024},
		"networks": {
			"eth0": {"rx_bytes": 10, "tx_bytes": 20},
			"eth1": {"rx_bytes": 5, "tx_bytes": 7}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}

	if stats.CPUPercent != 40 {
		t.Fatalf("expected CPU 40, got %f", stats.CPUPercent)
	}
	if stats.MemoryBytes != 512 || stats.MemoryLimit != 1024 {
		t.Fatalf("unexpected memory stats: %+v", stats)
	}
	if stats.NetworkRxBytes != 15 || stats.NetworkTxBytes != 27 {
		t.Fatalf("unexpected network stats: %+v", stats)
	}
}
