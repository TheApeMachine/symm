package utils

import (
	"context"
	"time"

	"github.com/theapemachine/symm/types"
)

type Waiter[T any] struct {
	ctx      context.Context
	cancel   context.CancelFunc
	reporter types.StatusReporter
	done     bool
}

func NewWaiter[T any](reporter types.StatusReporter) *Waiter[T] {
	ctx, cancel := context.WithCancel(context.Background())

	return &Waiter[T]{
		ctx:      ctx,
		cancel:   cancel,
		reporter: reporter,
		done:     false,
	}
}

func (waiter *Waiter[T]) Wait() T {
	for !waiter.done {
		select {
		case <-waiter.ctx.Done():
			waiter.done = true
		case <-time.After(100 * time.Millisecond):
			if waiter.reporter.Status() == types.READY {
				waiter.cancel()
			}
		}
	}

	return waiter.reporter.(T)
}
