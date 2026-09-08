//go:build darwin

package main

import (
	"encoding/binary"
	"math"
	"unsafe"

	"golang.org/x/sys/unix"
)

// sysLoadAvg mirrors the C struct loadavg on macOS/Darwin.
type sysLoadAvg struct {
	Exp        [3]int32 // load averages * fix(fscale)
	Fscale     int32
	Samples    int32
	Pad_cgo_0  [4]byte
	TimevalSec int64
}

func systemLoadAverage() float64 {
	raw, err := unix.SysctlRaw("vm.loadavg")
	if err != nil {
		return 0
	}
	if len(raw) < int(unsafe.Sizeof(sysLoadAvg{})) {
		return 0
	}
	var la sysLoadAvg
	// Use native byte order for the struct decoding.
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&la)), len(raw))
	copy(buf, raw)
	if la.Fscale == 0 {
		return 0
	}
	_ = binary.NativeEndian
	load1 := float64(la.Exp[0]) / float64(la.Fscale)
	if math.IsNaN(load1) || math.IsInf(load1, 0) {
		return 0
	}
	return load1
}
