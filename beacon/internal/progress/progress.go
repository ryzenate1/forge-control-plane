package progress

import "sync"

type Phase string

const (
	PhasePending    Phase = "pending"
	PhaseArchiving  Phase = "archiving"
	PhaseUploading  Phase = "uploading"
	PhaseDownloading Phase = "downloading"
	PhaseRestoring  Phase = "restoring"
	PhaseVerifying  Phase = "verifying"
	PhaseCompleted  Phase = "completed"
	PhaseFailed     Phase = "failed"
)

type Report struct {
	BytesProcessed int64 `json:"bytesProcessed"`
	TotalBytes     int64 `json:"totalBytes"`
	Phase          Phase `json:"phase"`
}

type Progress struct {
	mu       sync.RWMutex
	total    int64
	current  int64
	phase    Phase
	callback func(Report)
}

func New(callback func(Report)) *Progress {
	return &Progress{callback: callback}
}

func (p *Progress) SetTotal(total int64) {
	p.mu.Lock()
	p.total = total
	p.mu.Unlock()
}

func (p *Progress) SetPhase(phase Phase) {
	p.mu.Lock()
	p.phase = phase
	report := p.report()
	p.mu.Unlock()
	p.notify(report)
}

func (p *Progress) Add(n int64) {
	p.mu.Lock()
	p.current += n
	report := p.report()
	p.mu.Unlock()
	p.notify(report)
}

func (p *Progress) Report() Report {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.report()
}

func (p *Progress) report() Report {
	return Report{
		BytesProcessed: p.current,
		TotalBytes:     p.total,
		Phase:          p.phase,
	}
}

func (p *Progress) notify(r Report) {
	if p.callback != nil {
		p.callback(r)
	}
}
