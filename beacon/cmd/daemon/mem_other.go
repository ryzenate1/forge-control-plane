//go:build !linux

package main

import goruntime "runtime"

// readMemoryMB falls back to Go heap allocation on non-Linux platforms.
func readMemoryMB() int64 {
	var m goruntime.MemStats
	goruntime.ReadMemStats(&m)
	return int64(m.Sys / (1024 * 1024))
}
