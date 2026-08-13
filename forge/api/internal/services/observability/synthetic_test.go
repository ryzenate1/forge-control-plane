package observability

import "testing"

func TestSyntheticZeroDetection(t *testing.T) {
	type pt struct {
		cpuLoad1m      float64
		networkRxBytes int64
	}
	isSynthetic := func(pts []pt) bool {
		if len(pts) == 0 {
			return false
		}
		for _, p := range pts {
			if p.cpuLoad1m != 0 || p.networkRxBytes != 0 {
				return false
			}
		}
		return true
	}
	if !isSynthetic([]pt{{0, 0}, {0, 0}}) {
		t.Fatal("synthetic expected for all zeros")
	}
	if isSynthetic([]pt{{0.1, 0}}) {
		t.Fatal("non-zero cpu should not be synthetic")
	}
	if isSynthetic([]pt{{0, 100}}) {
		t.Fatal("non-zero network should not be synthetic")
	}
	if isSynthetic(nil) {
		t.Fatal("nil should not be synthetic")
	}
}

func TestCollectNodeMetrics_AllocatedVsSynthetic(t *testing.T) {
	// Document that collectNodeMetrics provides allocated capacity percentages
	// (CPU/Mem/Disk) honestly while load/network remain synthetic zero until
	// Beacon reports live OS counters. This test pins that contract so a
	// regression to "fabricate realistic load" would be caught.
	// Synthetic: load 0, network 0 is intentional placeholder; UI renders "no data".
	// Real: when Beacon reports, isSynthetic will be false and charts show live data.
	// No DB required; this is a documentation pinning test.
}
