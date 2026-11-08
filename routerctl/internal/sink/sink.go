// Package sink holds what the outputs (MQTT, notifications, metrics,
// stdout; the subpackages) share. The port they implement (Sink,
// (Sink, DeviceState, Control) belongs to the poller.
package sink

import (
	"context"
	"log/slog"
	"sync"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
)

// Async wraps a sink so State/Event never block the poll loop: calls are
// queued (in order) and handled by one goroutine. When the queue is full
// (the output is stuck) new items are dropped with a warning.
func Async(s poller.Sink, size int) poller.Sink {
	if size <= 0 {
		size = 256
	}
	return &async{inner: s, ch: make(chan func(), size), done: make(chan struct{})}
}

type async struct {
	inner   poller.Sink
	ch      chan func()
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	close   bool
	started bool
}

func (a *async) Name() string { return a.inner.Name() }

func (a *async) Start(ctx context.Context, ctl poller.Control) error {
	if err := a.inner.Start(ctx, ctl); err != nil {
		return err
	}
	a.mu.Lock()
	a.started = true
	a.mu.Unlock()
	go func() {
		defer close(a.done)
		for fn := range a.ch {
			fn()
		}
	}()
	return nil
}

func (a *async) push(fn func(), what string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.close {
		return
	}
	select {
	case a.ch <- fn:
	default:
		slog.Warn("sink queue full, dropping", "sink", a.inner.Name(), "item", what)
	}
}

func (a *async) State(st poller.DeviceState) { a.push(func() { a.inner.State(st) }, "state") }
func (a *async) Event(e events.Event)        { a.push(func() { a.inner.Event(e) }, string(e.Type)) }

// Close drains the queue (until ctx is done) and closes the inner sink.
func (a *async) Close(ctx context.Context) error {
	a.once.Do(func() {
		a.mu.Lock()
		a.close = true
		close(a.ch)
		if !a.started {
			close(a.done)
		}
		a.mu.Unlock()
	})
	select {
	case <-a.done:
	case <-ctx.Done():
	}
	return a.inner.Close(ctx)
}
