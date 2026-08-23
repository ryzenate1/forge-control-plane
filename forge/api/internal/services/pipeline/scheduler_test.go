package pipeline

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

// Unit tests for the schedule-trigger due-slot arithmetic (audit F-HIGH-5).
// The scheduler evaluates one minute-slot per tick; these tests pin the
// pure decision function so DB-free regressions are caught.

func mustCron(t *testing.T, expr string) cron.Schedule {
	t.Helper()
	s, err := cron.ParseStandard(expr)
	if err != nil {
		t.Fatalf("parse %q: %v", expr, err)
	}
	return s
}

func TestScheduleDueEveryMinute(t *testing.T) {
	sched := mustCron(t, "* * * * *")
	slot := time.Date(2026, 8, 23, 12, 30, 0, 0, time.UTC)
	if !scheduleDueInSlot(sched, slot) {
		t.Error("every-minute schedule should be due in any slot")
	}
}

func TestScheduleNotDueWrongHour(t *testing.T) {
	sched := mustCron(t, "0 0 1 1 *") // yearly Jan 1 00:00
	slot := time.Date(2026, 8, 23, 12, 30, 0, 0, time.UTC)
	if scheduleDueInSlot(sched, slot) {
		t.Error("yearly schedule must not fire in August")
	}
}

func TestScheduleDueAtExactHour(t *testing.T) {
	sched := mustCron(t, "30 12 * * *") // daily at 12:30
	dueSlot := time.Date(2026, 8, 23, 12, 30, 0, 0, time.UTC)
	if !scheduleDueInSlot(sched, dueSlot) {
		t.Error("daily 12:30 schedule should be due in the 12:30 slot")
	}
	wrongSlot := time.Date(2026, 8, 23, 12, 31, 0, 0, time.UTC)
	if scheduleDueInSlot(sched, wrongSlot) {
		t.Error("daily 12:30 schedule must not fire in the 12:31 slot")
	}
}
