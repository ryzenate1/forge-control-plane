//go:build !linux

package quota

func newTracker() Tracker {
	return NoopTracker{}
}
