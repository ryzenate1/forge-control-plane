package mail

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type Worker struct {
	store    *store.Store
	sender   SMTPSender
	workerID string
	wake     time.Duration
	wg       sync.WaitGroup
}

func NewWorker(s *store.Store) *Worker {
	return &Worker{store: s, sender: SMTPSender{Timeout: 15 * time.Second}, workerID: "mail-" + uuid.NewString(), wake: time.Second}
}
func (w *Worker) Start(ctx context.Context) {
	if w != nil && w.store != nil {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					buf := make([]byte, 4096)
					n := runtime.Stack(buf, false)
					slog.Error("mail worker panic", "panic", r, "stack", string(buf[:n]))
				}
			}()
			w.loop(ctx)
		}()
	}
}

func (w *Worker) Wait() {
	if w != nil {
		w.wg.Wait()
	}
}

func (w *Worker) loop(ctx context.Context) {
	ticker := time.NewTicker(w.wake)
	defer ticker.Stop()
	for {
		if !w.processOne(ctx) {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func (w *Worker) processOne(ctx context.Context) bool {
	claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	item, err := w.store.ClaimMail(claimCtx, w.workerID, time.Minute)
	cancel()
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("mail worker claim", "error", err)
		}
		return false
	}
	if item == nil {
		return false
	}
	settingsCtx, settingsCancel := context.WithTimeout(ctx, 5*time.Second)
	settings, err := w.store.GetPanelMailSettings(settingsCtx)
	settingsCancel()
	if err == nil {
		if settings.Driver == "log" {
			slog.Info("mail (log driver)", "to", item.Recipient, "subject", item.Subject)
		} else {
			sendCtx, sendCancel := context.WithTimeout(ctx, 20*time.Second)
			err = w.sender.Send(sendCtx, settings, item.Recipient, item.Subject, item.TextBody, item.HTMLBody)
			sendCancel()
		}
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	if err == nil {
		if e := w.store.CompleteMail(finishCtx, item.ID, w.workerID); e != nil {
			slog.Error("mail worker complete", "id", item.ID, "error", e)
		}
		return true
	}
	if e := w.store.RetryMail(finishCtx, item.ID, w.workerID, err.Error(), RetryDelay(item.Attempts)); e != nil {
		slog.Error("mail worker retry", "id", item.ID, "error", e)
	}
	return true
}

func (w *Worker) Enqueue(ctx context.Context, recipient, subject, textBody, htmlBody string) error {
	// The recipient becomes the SMTP envelope; CR/LF here would corrupt the
	// envelope, so reject instead of sanitizing (a mangled address must not
	// be delivered anywhere). The subject is a header: sanitize rather than
	// reject so a weird server name degrades to a flat subject line.
	if strings.ContainsAny(recipient, "\r\n") {
		return fmt.Errorf("mail: recipient contains newline")
	}
	subject = sanitizeHeaderValue(subject)
	_, err := w.store.EnqueueMail(ctx, recipient, subject, textBody, htmlBody)
	return err
}

func RetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	return time.Duration(1<<uint(attempt-1)) * time.Minute
}
