package server

import (
	"testing"
	"time"
)

func TestCrashDetectorShouldAutoRestart(t *testing.T) {
	cd := NewCrashDetector(CrashThresholds{
		MaxCrashesInWindow: 3,
		WindowDuration:     10 * time.Minute,
		CooldownDuration:   30 * time.Minute,
	})

	if !cd.ShouldAutoRestart("srv-1") {
		t.Fatal("expected auto restart with no crashes")
	}

	cd.RecordCrash("srv-1", 1, false)
	cd.RecordCrash("srv-1", 1, false)

	if !cd.ShouldAutoRestart("srv-1") {
		t.Fatal("expected auto restart with 2 crashes under threshold")
	}

	cd.RecordCrash("srv-1", 1, false)

	if cd.ShouldAutoRestart("srv-1") {
		t.Fatal("expected no auto restart after reaching threshold")
	}
}

func TestCrashDetectorCooldown(t *testing.T) {
	cd := NewCrashDetector(CrashThresholds{
		MaxCrashesInWindow: 2,
		WindowDuration:     10 * time.Minute,
		CooldownDuration:   100 * time.Millisecond,
	})

	cd.RecordCrash("srv-1", 1, false)
	cd.RecordCrash("srv-1", 1, false)

	if cd.ShouldAutoRestart("srv-1") {
		t.Fatal("expected no restart during cooldown")
	}

	time.Sleep(150 * time.Millisecond)

	if !cd.ShouldAutoRestart("srv-1") {
		t.Fatal("expected restart after cooldown expired")
	}
}

func TestCrashDetectorCleanExit(t *testing.T) {
	cd := NewCrashDetector(CrashThresholds{
		MaxCrashesInWindow: 2,
		WindowDuration:     10 * time.Minute,
		CooldownDuration:   30 * time.Minute,
	})

	cd.RecordCrash("srv-1", 0, true)
	cd.RecordCrash("srv-1", 0, true)

	if !cd.ShouldAutoRestart("srv-1") {
		t.Fatal("clean exits should not count by default")
	}

	cd.DetectCleanExitAsCrash = true

	cd.Reset("srv-1")
	cd.RecordCrash("srv-1", 0, true)
	cd.RecordCrash("srv-1", 0, true)

	if cd.ShouldAutoRestart("srv-1") {
		t.Fatal("clean exits should count when DetectCleanExitAsCrash is true")
	}
}

func TestCrashDetectorReset(t *testing.T) {
	cd := NewCrashDetector(DefaultCrashThresholds())
	cd.RecordCrash("srv-1", 1, false)
	cd.RecordCrash("srv-1", 1, false)
	cd.RecordCrash("srv-1", 1, false)

	cd.Reset("srv-1")

	if !cd.ShouldAutoRestart("srv-1") {
		t.Fatal("expected auto restart after reset")
	}
	h := cd.GetCrashHistory("srv-1")
	if len(h) != 0 {
		t.Fatalf("expected empty history after reset, got %d", len(h))
	}
}

func TestCrashDetectorGetHistory(t *testing.T) {
	cd := NewCrashDetector(DefaultCrashThresholds())
	cd.RecordCrash("srv-1", 1, false)
	cd.RecordCrash("srv-1", 137, false)

	h := cd.GetCrashHistory("srv-1")
	if len(h) != 2 {
		t.Fatalf("expected 2 records, got %d", len(h))
	}
	if h[0].ExitCode != 1 || h[1].ExitCode != 137 {
		t.Fatalf("unexpected exit codes: %d, %d", h[0].ExitCode, h[1].ExitCode)
	}
}

func TestCrashDetectorMarkAutoRestarted(t *testing.T) {
	cd := NewCrashDetector(DefaultCrashThresholds())
	cd.RecordCrash("srv-1", 1, false)
	cd.MarkAutoRestarted("srv-1")

	h := cd.GetCrashHistory("srv-1")
	if !h[0].AutoRestarted {
		t.Fatal("expected AutoRestarted to be true")
	}
}

func TestCrashDetectorDefaultThresholds(t *testing.T) {
	th := DefaultCrashThresholds()
	if th.MaxCrashesInWindow != 3 {
		t.Fatalf("expected MaxCrashesInWindow 3, got %d", th.MaxCrashesInWindow)
	}
	if th.WindowDuration != 10*time.Minute {
		t.Fatalf("expected WindowDuration 10m, got %v", th.WindowDuration)
	}
	if th.CooldownDuration != 30*time.Minute {
		t.Fatalf("expected CooldownDuration 30m, got %v", th.CooldownDuration)
	}
}

func TestCrashDetectorInvalidThresholds(t *testing.T) {
	cd := NewCrashDetector(CrashThresholds{})
	if cd.thresholds.MaxCrashesInWindow != 3 {
		t.Fatalf("expected default MaxCrashesInWindow, got %d", cd.thresholds.MaxCrashesInWindow)
	}
	if cd.thresholds.WindowDuration != 10*time.Minute {
		t.Fatalf("expected default WindowDuration, got %v", cd.thresholds.WindowDuration)
	}
}

func TestCrashDetectorGetHistoryReturnsCopy(t *testing.T) {
	cd := NewCrashDetector(DefaultCrashThresholds())
	cd.RecordCrash("srv-1", 1, false)

	h1 := cd.GetCrashHistory("srv-1")
	h1[0].ExitCode = 999

	h2 := cd.GetCrashHistory("srv-1")
	if h2[0].ExitCode == 999 {
		t.Fatal("GetCrashHistory should return a copy")
	}
}

func TestCrashDetectorWindowExpiration(t *testing.T) {
	cd := NewCrashDetector(CrashThresholds{
		MaxCrashesInWindow: 2,
		WindowDuration:     100 * time.Millisecond,
		CooldownDuration:   50 * time.Millisecond,
	})

	cd.RecordCrash("srv-1", 1, false)
	cd.RecordCrash("srv-1", 1, false)

	if cd.ShouldAutoRestart("srv-1") {
		t.Fatal("expected no restart at threshold")
	}

	time.Sleep(150 * time.Millisecond)

	if !cd.ShouldAutoRestart("srv-1") {
		t.Fatal("expected restart after window expired")
	}
}
