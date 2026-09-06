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

	// MaxCooldownEscalations is the ceiling of the crash-loop guard: how many
	// times a workload may trip the crash threshold before automatic restarts
	// stop entirely and an operator has to intervene (Reset). Without a ceiling
	// the guard is only a delay — the loop restarts forever, just slower.
	MaxCooldownEscalations int
}

// maxEscalatedCooldown bounds the exponential growth of the cooldown so a broken
// workload is not parked for weeks by a rule meant to slow a loop down.
const maxEscalatedCooldown = 6 * time.Hour

func DefaultCrashThresholds() CrashThresholds {
	return CrashThresholds{
		MaxCrashesInWindow:     3,
		WindowDuration:         10 * time.Minute,
		CooldownDuration:       30 * time.Minute,
		MaxCooldownEscalations: 5,
	}
}

type CrashDetector struct {
	mu                     sync.Mutex
	thresholds             CrashThresholds
	DetectCleanExitAsCrash bool
	records                map[string][]CrashRecord
	cooldowns              map[string]time.Time

	// escalations counts threshold trips that were never followed by an observed
	// healthy run. It drives both the growing cooldown and the circuit-open
	// ceiling, and it is deliberately NOT cleared when a cooldown expires:
	// clearing it there is what turned the breaker into a timer and let a crash
	// loop run forever.
	escalations map[string]int
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
	if thresholds.MaxCooldownEscalations < 1 {
		thresholds.MaxCooldownEscalations = 5
	}
	return &CrashDetector{
		thresholds:  thresholds,
		records:     make(map[string][]CrashRecord),
		cooldowns:   make(map[string]time.Time),
		escalations: make(map[string]int),
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

// trimOldRecords drops records outside the crash window and evicts map entries
// that trimming empties, so workloads removed long ago stop holding state
// forever.
func (cd *CrashDetector) trimOldRecords(serverID string) {
	cutoff := time.Now().Add(-cd.thresholds.WindowDuration)
	records := cd.records[serverID]
	start := 0
	for start < len(records) && records[start].Timestamp.Before(cutoff) {
		start++
	}
	if start > 0 {
		records = records[start:]
	}
	if len(records) == 0 {
		delete(cd.records, serverID)
		return
	}
	cd.records[serverID] = records
}

// ShouldAutoRestart reports whether a crashed workload may be restarted
// automatically. It decides on crash history only: it does not know whether an
// operator stopped the workload on purpose, so a true return is not licence to
// start something a human stopped. The caller owns that check.
func (cd *CrashDetector) ShouldAutoRestart(serverID string) bool {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	if cd.circuitOpen(serverID) {
		return false
	}

	if cooldownUntil, ok := cd.cooldowns[serverID]; ok {
		if time.Now().Before(cooldownUntil) {
			return false
		}
		// The cooldown elapsed: a fresh crash window starts. The escalation
		// counter keeps its memory, so repeated trips slow down and eventually
		// stop instead of restarting on the same cadence forever.
		delete(cd.cooldowns, serverID)
		delete(cd.records, serverID)
	}

	cd.trimOldRecords(serverID)

	crashCount := 0
	for _, r := range cd.records[serverID] {
		if !r.CleanExit || cd.DetectCleanExitAsCrash {
			crashCount++
		}
	}

	if crashCount >= cd.thresholds.MaxCrashesInWindow {
		cd.escalations[serverID]++
		if !cd.circuitOpen(serverID) {
			cd.cooldowns[serverID] = time.Now().Add(cd.escalatedCooldown(serverID))
		}
		return false
	}

	return true
}

// escalatedCooldown doubles the configured cooldown on every trip, capped at
// maxEscalatedCooldown.
func (cd *CrashDetector) escalatedCooldown(serverID string) time.Duration {
	trips := cd.escalations[serverID]
	if trips < 1 {
		trips = 1
	}
	cooldown := cd.thresholds.CooldownDuration
	for i := 1; i < trips; i++ {
		if cooldown >= maxEscalatedCooldown/2 {
			return maxEscalatedCooldown
		}
		cooldown *= 2
	}
	if cooldown > maxEscalatedCooldown {
		return maxEscalatedCooldown
	}
	return cooldown
}

func (cd *CrashDetector) circuitOpen(serverID string) bool {
	return cd.escalations[serverID] >= cd.thresholds.MaxCooldownEscalations
}

// CircuitOpen reports the breaker state so a caller can say "this workload is
// not being restarted any more" instead of leaving a dead workload looking like
// it is merely between restart attempts.
func (cd *CrashDetector) CircuitOpen(serverID string) bool {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	return cd.circuitOpen(serverID)
}

// CooldownUntil reports when a tripped workload becomes eligible again. ok is
// false when it is not cooling down — or when the breaker has opened, in which
// case there is no next attempt to promise and the caller must say so.
func (cd *CrashDetector) CooldownUntil(serverID string) (until time.Time, ok bool) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	if cd.circuitOpen(serverID) {
		return time.Time{}, false
	}
	cooldownUntil, exists := cd.cooldowns[serverID]
	if !exists {
		return time.Time{}, false
	}
	return cooldownUntil, true
}

// MarkHealthyObserved clears the crash-loop memory for a workload that has been
// *observed running*, not merely issued a start. A restart that has not been
// seen healthy must not reset the counter, or a boot-crash loop whose start
// succeeds and dies within seconds resets itself on every attempt and never
// reaches the ceiling.
func (cd *CrashDetector) MarkHealthyObserved(serverID string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	delete(cd.records, serverID)
	delete(cd.cooldowns, serverID)
	delete(cd.escalations, serverID)
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

// Reset is operator intervention: it forgets the crash history and closes the
// circuit again. It must not be called on a restart attempt — only when a human
// or equivalent authority decides to give the workload another chance.
func (cd *CrashDetector) Reset(serverID string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	delete(cd.records, serverID)
	delete(cd.cooldowns, serverID)
	delete(cd.escalations, serverID)
}
