package system

import "sync/atomic"

// AtomicString provides atomic string operations
type AtomicString struct {
	v atomic.Value
}

// NewAtomicString creates a new atomic string
func NewAtomicString(initial string) *AtomicString {
	a := &AtomicString{}
	a.Store(initial)
	return a
}

// Load returns the current value
func (a *AtomicString) Load() string {
	v := a.v.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

// Store sets the value
func (a *AtomicString) Store(val string) {
	a.v.Store(val)
}

// AtomicBool provides atomic boolean operations
type AtomicBool struct {
	v atomic.Value
}

// NewAtomicBool creates a new atomic bool
func NewAtomicBool(initial bool) *AtomicBool {
	a := &AtomicBool{}
	a.Store(initial)
	return a
}

// Load returns the current value
func (a *AtomicBool) Load() bool {
	v := a.v.Load()
	if v == nil {
		return false
	}
	return v.(bool)
}

// Store sets the value
func (a *AtomicBool) Store(val bool) {
	a.v.Store(val)
}

// SwapIf swaps to true if currently false, returns success
func (a *AtomicBool) SwapIf(val bool) bool {
	current := a.Load()
	if current == !val {
		a.Store(val)
		return true
	}
	return false
}
