//go:build !unix

package metrics

// processCPUTimes reports zero CPU usage on platforms without getrusage
// (for example Windows); the daemon only targets unix-like hosts today.
func processCPUTimes() (userSeconds, systemSeconds float64) {
	return 0, 0
}
