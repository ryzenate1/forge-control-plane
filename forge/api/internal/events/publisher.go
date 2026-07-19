package events

import (
	"context"
	"time"
)

type Publisher interface {
	Publish(context.Context, Envelope) error
}

type PublisherMiddleware func(Publisher) Publisher

type PublisherFunc func(context.Context, Envelope) error

func (f PublisherFunc) Publish(ctx context.Context, event Envelope) error {
	return f(ctx, event)
}

func ApplyPublisherMiddleware(p Publisher, middleware ...PublisherMiddleware) Publisher {
	for i := len(middleware) - 1; i >= 0; i-- {
		p = middleware[i](p)
	}
	return p
}

type DurationProvider func() time.Duration

func DurationProviderFrom(d time.Duration) DurationProvider {
	return func() time.Duration { return d }
}

func PublisherWithRetry(maxRetries int, baseWait DurationProvider) PublisherMiddleware {
	return func(next Publisher) Publisher {
		return PublisherFunc(func(ctx context.Context, event Envelope) error {
			var lastErr error
			wait := baseWait()
			for attempt := 0; attempt <= maxRetries; attempt++ {
				if attempt > 0 {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(wait):
					}
					wait *= 2
				}
				if err := next.Publish(ctx, event); err != nil {
					lastErr = err
					continue
				}
				return nil
			}
			return lastErr
		})
	}
}
