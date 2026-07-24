package types

import (
	"context"
	"sync"
	"time"
)

/*
ReadyFuture resolves once when a component reaches READY or fails. Waiters
share one signal so boot stages do not busy-poll forever without a deadline.
*/
type ReadyFuture struct {
	once sync.Once
	done chan struct{}
	err  error
}

/*
NewReadyFuture allocates an unresolved readiness future.
*/
func NewReadyFuture() *ReadyFuture {
	return &ReadyFuture{done: make(chan struct{})}
}

/*
Resolve marks the future ready or failed. Only the first call wins.
*/
func (future *ReadyFuture) Resolve(err error) {
	if future == nil {
		return
	}

	future.once.Do(func() {
		future.err = err
		close(future.done)
	})
}

/*
Wait blocks until the future resolves or the context ends.
*/
func (future *ReadyFuture) Wait(ctx context.Context) error {
	if future == nil {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-future.done:
		return future.err
	}
}

/*
WaitStatus polls a StatusReporter until READY, ERROR, or context cancellation.
Prefer ReadyFuture when a component exposes one; this remains for reporters
that only publish Status snapshots.
*/
func WaitStatus(ctx context.Context, reporter StatusReporter) error {
	if reporter == nil {
		return nil
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		switch reporter.Status() {
		case READY:
			return nil
		case ERROR, FATAL:
			return ClosedError{Component: "status"}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
