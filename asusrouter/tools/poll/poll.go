// Package poll ports `asusrouter/tools/poll`: polling tools.
package poll

import (
	"context"
	"time"
)

// Python defaults of `async_poll_until`
const (
	DefaultInterval = time.Second
	DefaultAttempts = 5
)

// sleep waits for the interval or the context (replaced in tests).
var sleep = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// AsyncPollUntil mirrors `async_poll_until`: call probe until
// ready(result) is true, else nil after attempts.
//
// Python's `**kwargs` for the probe become a closure. A probe error
// stops polling and is returned (Python propagates the exception), as is
// a context error while waiting.
func AsyncPollUntil[T any](
	ctx context.Context,
	probe func(context.Context) (T, error),
	ready func(T) bool,
	interval time.Duration,
	attempts int,
) (*T, error) {
	for attempt := range attempts {
		result, err := probe(ctx)
		if err != nil {
			return nil, err
		}
		if ready(result) {
			return &result, nil
		}
		if attempt+1 < attempts {
			if err := sleep(ctx, interval); err != nil {
				return nil, err
			}
		}
	}

	return nil, nil
}
