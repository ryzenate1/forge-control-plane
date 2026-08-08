package queue

import (
	"math"
	"math/rand/v2"
	"time"
)

type ClientRetryPolicy interface {
	NextRetry(job *JobRow) time.Time
}

type DefaultClientRetryPolicy struct {
	timeNowFunc func() time.Time
}

func (p *DefaultClientRetryPolicy) NextRetry(job *JobRow) time.Time {
	errorCount := len(job.Errors) + 1
	return p.timeNowUTC().Add(secondsAsDuration(p.retrySeconds(errorCount)))
}

func (p *DefaultClientRetryPolicy) timeNowUTC() time.Time {
	if p.timeNowFunc != nil {
		return p.timeNowFunc()
	}
	return time.Now().UTC()
}

const maxDuration time.Duration = 1<<63 - 1

var maxDurationSeconds = maxDuration.Seconds()

func (p *DefaultClientRetryPolicy) retrySeconds(attempt int) float64 {
	retrySeconds := p.retrySecondsWithoutJitter(attempt)
	if retrySeconds == maxDurationSeconds {
		return maxDurationSeconds
	}
	retrySeconds += retrySeconds * (rand.Float64()*0.2 - 0.1)
	return min(retrySeconds, maxDurationSeconds)
}

func (p *DefaultClientRetryPolicy) retrySecondsWithoutJitter(attempt int) float64 {
	retrySeconds := math.Pow(float64(attempt), 4)
	if retrySeconds > maxDurationSeconds {
		return maxDurationSeconds
	}
	return retrySeconds
}

func secondsAsDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
