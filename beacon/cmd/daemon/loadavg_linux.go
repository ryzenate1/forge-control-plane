//go:build linux

package main

import (
	"os"
	"strconv"
	"strings"
)

func systemLoadAverage() float64 {
	body, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return 0
	}
	value, _ := strconv.ParseFloat(fields[0], 64)
	return value
}
