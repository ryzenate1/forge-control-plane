package system

import (
	"context"
	"errors"
	"sync"
)

// ErrLockerLocked is returned when attempting to acquire a lock that is
// already held.
var ErrLockerLocked = errors.New("locker: cannot acquire lock, already locked")
var ErrLockerDestroyed = errors.New("locker: has been destroyed")

// Locker provides a channel-based lock that returns an error immediately if
// already locked rather than blocking. This non-blocking behaviour is critical
// for the console throttle's strike-once pattern.
type Locker struct {
	mu        sync.RWMutex
	ch        chan bool
	destroyed bool
}

// NewLocker returns a new Locker instance.
func NewLocker() *Locker {
	return &Locker{
		ch: make(chan bool, 1),
	}
}

// IsLocked returns the current state of the locker channel. If there is
// currently a value in the channel, it is assumed to be locked.
func (l *Locker) IsLocked() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.ch) == 1
}

// Acquire will acquire the power lock if it is not currently locked. If it is
// already locked, acquire will fail to acquire the lock, and will return an error.
func (l *Locker) Acquire() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.destroyed {
		return ErrLockerDestroyed
	}
	select {
	case l.ch <- true:
	default:
		return ErrLockerLocked
	}
	return nil
}

// TryAcquire will attempt to acquire a power-lock until the context provided
// is canceled.
func (l *Locker) TryAcquire(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.destroyed {
		return ErrLockerDestroyed
	}
	select {
	case l.ch <- true:
		return nil
	default:
		return ErrLockerLocked
	}
}

// Release will drain the locker channel so that we can properly re-acquire it
// at a later time. If the channel is not currently locked this function is a
// no-op and will immediately return.
func (l *Locker) Release() {
	l.mu.Lock()
	if l.destroyed {
		l.mu.Unlock()
		return
	}
	select {
	case <-l.ch:
	default:
	}
	l.mu.Unlock()
}

// Destroy cleans up the power locker by closing the channel.
func (l *Locker) Destroy() {
	l.mu.Lock()
	if !l.destroyed {
		select {
		case <-l.ch:
		default:
		}
		l.destroyed = true
	}
	l.mu.Unlock()
}
