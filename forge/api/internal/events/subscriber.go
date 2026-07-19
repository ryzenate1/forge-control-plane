package events

import "context"

type Subscriber interface {
	Handle(context.Context, Envelope) error
}

type HandlerFunc func(context.Context, Envelope) error

func (f HandlerFunc) Handle(ctx context.Context, event Envelope) error {
	return f(ctx, event)
}

type SubscriberMiddleware func(Subscriber) Subscriber

func ApplySubscriberMiddleware(s Subscriber, middleware ...SubscriberMiddleware) Subscriber {
	for i := len(middleware) - 1; i >= 0; i-- {
		s = middleware[i](s)
	}
	return s
}

func SubscriberWithRecovery(next Subscriber) Subscriber {
	return HandlerFunc(func(ctx context.Context, event Envelope) (err error) {
		defer func() {
			if r := recover(); r != nil {
				if recoveredErr, ok := r.(error); ok {
					err = recoveredErr
				} else {
					err = context.DeadlineExceeded
				}
			}
		}()
		return next.Handle(ctx, event)
	})
}
