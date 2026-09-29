package previewenv

import (
	"context"
	"runtime"
	"time"
)

// reapBatchSize caps how many rows a single reaper pass takes on. Backlogs are
// worked off over several ticks instead of one unbounded query.
const reapBatchSize = 200

// Reaper implements the auto-destroy half of the preview lifecycle: every
// 5 minutes live previews whose expires_at (created + PREVIEW_TTL) has
// elapsed are cleaned up, and cleaned_up rows older than RetainCleaned are
// deleted outright (row expiry). It lives in this package so the shared
// cleanup service (reservations/allocations) stays untouched.
type reaper struct {
	svc  *Service
	done chan struct{}
}

// Start launches the reaper goroutine; it stops when ctx is cancelled.
// Idempotent and safe to call from the registrar at startup.
func (s *Service) StartReaper(ctx context.Context, interval time.Duration) *reaper {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	r := &reaper{svc: s, done: make(chan struct{})}
	go r.loop(ctx, interval)
	return r
}

func (r *reaper) loop(ctx context.Context, interval time.Duration) {
	// done is closed on the way out no matter how the loop ends, so Stop can
	// never block forever on a goroutine that already died.
	defer close(r.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// The guard is per pass: a panic that escaped the loop would end
			// reaping for the rest of the process lifetime, silently leaking
			// every preview created afterwards.
			r.runPassSafely(ctx)
		}
	}
}

func (r *reaper) runPassSafely(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			if r.svc.opts.Logger != nil {
				r.svc.opts.Logger.Error("previewenv reaper panic recovered; this pass was abandoned",
					"panic", rec, "stack", string(buf[:n]))
			}
		}
	}()
	r.runOnce(ctx)
}

func (r *reaper) runOnce(ctx context.Context) {
	srv := r.svc
	if srv == nil || srv.store == nil {
		return
	}
	now := time.Now().UTC()

	expired, err := srv.store.ListExpiredPreviewDeployments(ctx, now, reapBatchSize)
	if err != nil {
		if srv.opts.Logger != nil {
			srv.opts.Logger.Warn("previewenv reaper: list expired failed", "err", err)
		}
	}
	// A failed item is logged and the pass continues: one bad row must not
	// strand every preview behind it in the list.
	for i := range expired {
		p := &expired[i]
		if err := srv.Cleanup(ctx, p.ID); err != nil && srv.opts.Logger != nil {
			srv.opts.Logger.Warn("previewenv reaper: TTL cleanup failed",
				"preview", p.ID, "expiresAt", p.ExpiresAt, "err", err)
		}
	}

	reapable, err := srv.store.ListReapablePreviewDeployments(ctx, now.Add(-srv.opts.RetainCleaned), reapBatchSize)
	if err != nil && srv.opts.Logger != nil {
		srv.opts.Logger.Warn("previewenv reaper: list reapable failed", "err", err)
	}
	for i := range reapable {
		p := &reapable[i]
		if err := srv.store.DeletePreviewDeployment(ctx, p.ID); err != nil && srv.opts.Logger != nil {
			srv.opts.Logger.Warn("previewenv reaper: row expiry delete failed",
				"preview", p.ID, "err", err)
		}
	}
}

// Stop terminates a running reaper, waiting for the current pass to finish.
func (r *reaper) Stop() {
	if r == nil {
		return
	}
	<-r.done
}