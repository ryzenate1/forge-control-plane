package progress

import (
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	p := New(nil)
	if p == nil {
		t.Fatal("New returned nil")
	}
	r := p.Report()
	if r.Phase != "" {
		t.Fatalf("expected empty phase, got %s", r.Phase)
	}
}

func TestSetTotal(t *testing.T) {
	p := New(nil)
	p.SetTotal(1000)
	r := p.Report()
	if r.TotalBytes != 1000 {
		t.Fatalf("expected total 1000, got %d", r.TotalBytes)
	}
}

func TestSetPhase(t *testing.T) {
	var lastReport Report
	p := New(func(r Report) {
		lastReport = r
	})
	p.SetPhase(PhaseArchiving)
	if lastReport.Phase != PhaseArchiving {
		t.Fatalf("expected phase archiving, got %s", lastReport.Phase)
	}
	r := p.Report()
	if r.Phase != PhaseArchiving {
		t.Fatalf("expected phase archiving, got %s", r.Phase)
	}
}

func TestAdd(t *testing.T) {
	var reports []Report
	p := New(func(r Report) {
		reports = append(reports, r)
	})
	p.SetTotal(100)
	p.Add(30)
	p.Add(50)
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}
	if reports[0].BytesProcessed != 30 {
		t.Fatalf("expected 30, got %d", reports[0].BytesProcessed)
	}
	if reports[1].BytesProcessed != 80 {
		t.Fatalf("expected 80, got %d", reports[1].BytesProcessed)
	}
	r := p.Report()
	if r.BytesProcessed != 80 {
		t.Fatalf("expected 80, got %d", r.BytesProcessed)
	}
}

func TestPhaseTransitions(t *testing.T) {
	p := New(nil)
	p.SetPhase(PhasePending)
	p.SetPhase(PhaseArchiving)
	p.SetPhase(PhaseUploading)
	p.SetPhase(PhaseCompleted)
	r := p.Report()
	if r.Phase != PhaseCompleted {
		t.Fatalf("expected completed, got %s", r.Phase)
	}
}

func TestNilCallback(t *testing.T) {
	p := New(nil)
	p.SetTotal(100)
	p.SetPhase(PhaseArchiving)
	p.Add(50)
	// Should not panic
}

func TestConcurrentAccess(t *testing.T) {
	p := New(nil)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.SetTotal(1000)
			p.SetPhase(PhaseArchiving)
			p.Add(100)
			p.Report()
		}()
	}
	wg.Wait()
	r := p.Report()
	if r.BytesProcessed != 1000 {
		t.Fatalf("expected 1000, got %d", r.BytesProcessed)
	}
}

func TestReportSnapshot(t *testing.T) {
	p := New(nil)
	p.SetTotal(500)
	p.SetPhase(PhaseRestoring)
	p.Add(250)
	r := p.Report()
	if r.BytesProcessed != 250 {
		t.Fatalf("expected 250, got %d", r.BytesProcessed)
	}
	if r.TotalBytes != 500 {
		t.Fatalf("expected 500, got %d", r.TotalBytes)
	}
	if r.Phase != PhaseRestoring {
		t.Fatalf("expected restoring, got %s", r.Phase)
	}
}

func TestAllPhases(t *testing.T) {
	phases := []Phase{PhasePending, PhaseArchiving, PhaseUploading, PhaseDownloading, PhaseRestoring, PhaseVerifying, PhaseCompleted, PhaseFailed}
	for _, phase := range phases {
		p := New(nil)
		p.SetPhase(phase)
		r := p.Report()
		if r.Phase != phase {
			t.Fatalf("expected phase %s, got %s", phase, r.Phase)
		}
	}
}
