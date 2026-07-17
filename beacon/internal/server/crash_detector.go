package server

import (
	"sync"
	"time"
)

type CrashRecord struct {
	Timestamp     time.Time `json:"timestamp"`
	ExitCode      int       `json:"exitCode"`
	CleanExit     bool      `json:"cleanExit"`
	AutoRestarted bool      `json:"autoRestarted"`
}

type CrashThresholds struct {
	MaxCrashesInWindow int
	WindowDuration     time.Duration
	CooldownDuration   time.Duration
}

func DefaultCrashThresholds() CrashThresholds {
	return CrashThresholds{
		MaxCrashesInWindow: 3,
		WindowDuration:     10 * time.Minute,
		CooldownDuration:   30 * time.Minute,
	}
}

type CrashDetector struct {
	mu                     sync.Mutex
	thresholds             CrashThresholds
	DetectCleanExitAsCrash bool
	records                map[string][]CrashRecord
	cooldowns              map[string]time.Time
}

func NewCrashDetector(thresholds CrashThresholds) *CrashDetector {
	if thresholds.MaxCrashesInWindow < 1 {
		thresholds.MaxCrashesInWindow = 3
	}
	if thresholds.WindowDuration <= 0 {
		thresholds.WindowDuration = 10 * time.Minute
	}
	if thresholds.CooldownDuration <= 0 {
		thresholds.CooldownDuration = 30 * time.Minute
	}
	return &CrashDetector{
		thresholds: thresholds,
		records:    make(map[string][]CrashRecord),
		cooldowns:  make(map[string]time.Time),
	}
}

func (cd *CrashDetector) RecordCrash(serverID string, exitCode int, cleanExit bool) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	record := CrashRecord{
		Timestamp: time.Now(),
		ExitCode:  exitCode,
		CleanExit: cleanExit,
	}

	cd.records[serverID] = append(cd.records[serverID], record)
	cd.trimOldRecords(serverID)
}

func (cd *CrashDetector) trimOldRecords(serverID string) {
	cutoff := time.Now().Add(-cd.thresholds.WindowDuration)
	records := cd.records[serverID]
	start := 0
	for start < len(records) && records[start].Timestamp.Before(cutoff) {
		start++
	}
	if start > 0 {
		cd.records[serverID] = records[start:]
	}
}

func (cd *CrashDetector) ShouldAutoRestart(serverID string) bool {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	if cooldownUntil, ok := cd.cooldowns[serverID]; ok {
		if time.Now().Before(cooldownUntil) {
			return false
		}
		delete(cd.cooldowns, serverID)
		delete(cd.records, serverID)
	}

	cd.trimOldRecords(serverID)
	records := cd.records[serverID]

	crashCount := 0
	for _, r := range records {
		if !r.CleanExit || cd.DetectCleanExitAsCrash {
			crashCount++
		}
	}

	if crashCount >= cd.thresholds.MaxCrashesInWindow {
		cd.cooldowns[serverID] = time.Now().Add(cd.thresholds.CooldownDuration)
		return false
	}

	return true
}

func (cd *CrashDetector) MarkAutoRestarted(serverID string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	records := cd.records[serverID]
	if len(records) > 0 {
		records[len(records)-1].AutoRestarted = true
	}
}

func (cd *CrashDetector) GetCrashHistory(serverID string) []CrashRecord {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	records := cd.records[serverID]
	result := make([]CrashRecord, len(records))
	copy(result, records)
	return result
}

func (cd *CrashDetector) Reset(serverID string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	delete(cd.records, serverID)
	delete(cd.cooldowns, serverID)
}
