//go:build !linux

package main

func systemLoadAverage() float64 { return 0 }
