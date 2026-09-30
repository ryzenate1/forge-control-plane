package queue

import (
	"crypto/sha256"
	"encoding/binary"
	"time"
)

const (
	bitAvailable = 7
	bitCancelled = 6
	bitCompleted = 5
	bitDiscarded = 4
	bitPending   = 3
	bitRetryable = 2
	bitRunning   = 1
	bitScheduled = 0
)

func stateToBit(state JobState) int {
	switch state {
	case JobStateAvailable:
		return bitAvailable
	case JobStateCancelled:
		return bitCancelled
	case JobStateCompleted:
		return bitCompleted
	case JobStateDiscarded:
		return bitDiscarded
	case JobStatePending:
		return bitPending
	case JobStateRetryable:
		return bitRetryable
	case JobStateRunning:
		return bitRunning
	case JobStateScheduled:
		return bitScheduled
	default:
		return -1
	}
}

func bitToState(bit int) JobState {
	switch bit {
	case bitAvailable:
		return JobStateAvailable
	case bitCancelled:
		return JobStateCancelled
	case bitCompleted:
		return JobStateCompleted
	case bitDiscarded:
		return JobStateDiscarded
	case bitPending:
		return JobStatePending
	case bitRetryable:
		return JobStateRetryable
	case bitRunning:
		return JobStateRunning
	case bitScheduled:
		return JobStateScheduled
	default:
		return ""
	}
}

func UniqueStatesToBitmask(states []JobState) byte {
	var mask byte
	for _, state := range states {
		if bit := stateToBit(state); bit >= 0 {
			mask |= 1 << bit
		}
	}
	return mask
}

func UniqueStatesFromBitmask(mask byte) []JobState {
	var states []JobState
	for bit := 0; bit < 8; bit++ {
		if mask&(1<<bit) != 0 {
			if state := bitToState(bit); state != "" {
				states = append(states, state)
			}
		}
	}
	return states
}

func UniqueOptsByStateDefault() []JobState {
	return []JobState{
		JobStateAvailable,
		JobStateCompleted,
		JobStatePending,
		JobStateRetryable,
		JobStateRunning,
		JobStateScheduled,
	}
}

func UniqueKey(kind string, args []byte, opts *UniqueOpts) ([]byte, byte) {
	return uniqueKeyWithQueue(kind, args, kind, opts)
}

func uniqueKeyWithQueue(kind string, args []byte, queue string, opts *UniqueOpts) ([]byte, byte) {
	h := sha256.New()

	if opts != nil && !opts.ExcludeKind {
		h.Write([]byte(kind))
	}

	if opts != nil && opts.ByArgs {
		h.Write(args)
	}

	if opts != nil && opts.ByQueue {
		h.Write([]byte(queue))
	}

	if opts != nil && opts.ByPeriod > 0 {
		periodBytes := make([]byte, 8)
		bucket := time.Now().UTC().Truncate(opts.ByPeriod).UnixNano()
		binary.LittleEndian.PutUint64(periodBytes, uint64(bucket))
		h.Write(periodBytes)
	}

	states := UniqueOptsByStateDefault()
	if opts != nil && len(opts.ByState) > 0 {
		states = opts.ByState
	}
	bitmask := UniqueStatesToBitmask(states)

	return h.Sum(nil), bitmask
}
