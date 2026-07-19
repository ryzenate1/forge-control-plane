package throttle

import (
	"gamepanel/beacon/internal/system"
	"time"
)

type Config struct {
	Enabled bool
	Lines   uint64
	Period  uint64
}

func (c Config) withDefaults() Config {
	if c.Lines == 0 {
		c.Lines = 2000
	}
	if c.Period == 0 {
		c.Period = 100
	}
	return c
}

type ConsoleThrottle struct {
	rate    *system.Rate
	lock    *system.Locker
	strike  func()
	enabled bool
}

func NewConsoleThrottle(config Config, strikeCallback func()) *ConsoleThrottle {
	config = config.withDefaults()
	return &ConsoleThrottle{
		rate:    system.NewRate(config.Lines, time.Duration(config.Period)*time.Millisecond),
		lock:    system.NewLocker(),
		strike:  strikeCallback,
		enabled: config.Enabled,
	}
}

func (ct *ConsoleThrottle) Allow() bool {
	if !ct.enabled {
		return true
	}
	if !ct.rate.Try() {
		if err := ct.lock.Acquire(); err == nil {
			if ct.strike != nil {
				ct.strike()
			}
		}
		return false
	}
	ct.lock.Release()
	return true
}

func (ct *ConsoleThrottle) Reset() {
	ct.rate.Reset()
}
