package queue

import (
	"log/slog"
	"time"
)

type Config struct {
	Queues    map[string]QueueConfig
	Workers   *Workers
	RetryPolicy ClientRetryPolicy
	Logger    *slog.Logger
	MaxAttempts int
	JobTimeout   time.Duration
	PollOnly bool
	FetchPollInterval time.Duration
	FetchCooldown     time.Duration
	Schema string
}

type QueueConfig struct {
	MaxWorkers int
}
