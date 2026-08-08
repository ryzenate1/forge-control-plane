package queue

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

type Notifier struct {
	listener  Listener
	connMu    sync.Mutex
	connected bool
	logger    *slog.Logger
}

func NewNotifier(listener Listener, logger *slog.Logger) *Notifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &Notifier{
		listener: listener,
		logger:   logger,
	}
}

func (n *Notifier) Listen(ctx context.Context, topic string, callback func(*Notification)) error {
	n.connMu.Lock()
	if !n.connected {
		if err := n.listener.Connect(ctx); err != nil {
			n.connMu.Unlock()
			return err
		}
		n.connected = true
	}
	n.connMu.Unlock()

	if err := n.listener.Listen(ctx, topic); err != nil {
		return err
	}

	go n.listenLoop(ctx, topic, callback)
	return nil
}

func (n *Notifier) listenLoop(ctx context.Context, topic string, callback func(*Notification)) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			nr := runtime.Stack(buf, false)
			n.logger.Error("notifier listen loop panic recovered",
				"topic", topic, "panic", r, "stack", string(buf[:nr]))
		}
	}()
	backoff := 100 * time.Millisecond
	for {
		notification, err := n.listener.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			n.logger.Error("notifier: wait for notification failed",
				"topic", topic, "error", err)
			if err := n.reconnect(ctx, topic); err != nil {
				n.logger.Warn("notifier reconnect failed", "topic", topic, "error", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				if backoff < 5*time.Second {
					backoff *= 2
				}
				continue
			}
			backoff = 100 * time.Millisecond
			continue
		}
		if notification.Topic == topic {
			callback(notification)
		}
	}
}

func (n *Notifier) reconnect(ctx context.Context, topic string) error {
	n.connMu.Lock()
	defer n.connMu.Unlock()
	_ = n.listener.Close(ctx)
	n.connected = false
	if err := n.listener.Connect(ctx); err != nil {
		return err
	}
	if err := n.listener.Listen(ctx, topic); err != nil {
		_ = n.listener.Close(ctx)
		return err
	}
	n.connected = true
	return nil
}

func (n *Notifier) Unlisten(ctx context.Context, topic string) error {
	return n.listener.Unlisten(ctx, topic)
}

func (n *Notifier) Close(ctx context.Context) error {
	n.connMu.Lock()
	defer n.connMu.Unlock()
	if n.connected {
		n.connected = false
		return n.listener.Close(ctx)
	}
	return nil
}
