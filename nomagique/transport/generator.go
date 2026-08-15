package transport

import (
	"context"
	"iter"

	"github.com/theapemachine/errnie"
)

type Generator[T any] struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      errnie.ErrnieError
	iterator iter.Seq[T]
}

func NewGenerator[T any](
	ctx context.Context, iterator iter.Seq[T],
) *Generator[T] {
	ctx, cancel := context.WithCancel(ctx)

	return &Generator[T]{
		ctx:      ctx,
		cancel:   cancel,
		iterator: iterator,
	}
}

func (generator *Generator[T]) pull() iter.Seq[T] {
	return func(yield func(T) bool) {
		for k := range generator.iterator {
			if !yield(k) {
				return
			}
		}
	}
}

func (generator *Generator[T]) Generate(iterator *iter.Seq[T]) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)
		for {
			select {
			case <-generator.ctx.Done():
				return
			default:
				for k := range generator.pull() {
					select {
					case <-generator.ctx.Done():
						return
					case out <- k:
					}
				}

				return
			}
		}
	}()

	return out
}

/*
Error implements the error interface, which allows the Generator
to be used as an error type in the event of a failure.
*/
func (generator *Generator[T]) Error() string {
	return generator.err.Error()
}
