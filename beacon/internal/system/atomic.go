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
	v int32
}

// NewAtomicBool creates a new atomic bool
func NewAtomicBool(initial bool) *AtomicBool {
	a := &AtomicBool{}
	if initial {
		a.v = 1
	}
	return a
}

// Load returns the current value
func (a *AtomicBool) Load() bool {
	return atomic.LoadInt32(&a.v) != 0
}

// Store sets the value
func (a *AtomicBool) Store(val bool) {
	if val {
		atomic.StoreInt32(&a.v, 1)
	} else {
		atomic.StoreInt32(&a.v, 0)
	}
}

// SwapIf swaps to the given value if currently the opposite, returns success
func (a *AtomicBool) SwapIf(val bool) bool {
	var wanted int32
	if val {
		wanted = 1
	}
	return atomic.CompareAndSwapInt32(&a.v, 1-wanted, wanted)
}
