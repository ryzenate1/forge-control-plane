package build

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"
)

const base36Alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

var buildIDState struct {
	sync.Mutex
	lastMillis uint64
	sequence   uint64
}

func GenerateBuildID() string {
	const sequenceSpace = uint64(36 * 36 * 36 * 36 * 36)
	buildIDState.Lock()
	defer buildIDState.Unlock()

	millis := uint64(time.Now().UnixMilli())
	if millis < buildIDState.lastMillis {
		millis = buildIDState.lastMillis
	}
	if millis > buildIDState.lastMillis {
		var seed [8]byte
		if _, err := rand.Read(seed[:]); err != nil {
			panic("secure random source unavailable: " + err.Error())
		}
		buildIDState.lastMillis = millis
		buildIDState.sequence = binary.BigEndian.Uint64(seed[:]) % sequenceSpace
	} else {
		buildIDState.sequence++
		if buildIDState.sequence >= sequenceSpace {
			buildIDState.lastMillis++
			buildIDState.sequence = 0
		}
	}
	return encodeBase36(buildIDState.lastMillis, 9) + encodeBase36(buildIDState.sequence, 5)
}

func encodeBase36(value uint64, minWidth int) string {
	digits := make([]byte, 0, 13)
	for value > 0 {
		digits = append(digits, base36Alphabet[value%36])
		value /= 36
	}
	for len(digits) < minWidth {
		digits = append(digits, base36Alphabet[0])
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
