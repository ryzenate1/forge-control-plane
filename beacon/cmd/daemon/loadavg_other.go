//go:build !linux && !darwin

package main

func systemLoadAverage() float64 { return 0 }
