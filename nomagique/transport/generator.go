package transport

import (
	"context"
	"iter"
)

/*
Generator adapts an iterator to a cancellable channel. Ordered hot paths should
prefer RingBuffer; Generator remains useful at asynchronous system boundaries.
*/
type Generator[Value any] struct {
	context context.Context
	cancel  context.CancelFunc
	source  iter.Seq[Value]
}

/*
NewGenerator creates a cancellable iterator adapter.
*/
func NewGenerator[Value any](
	parent context.Context,
	source iter.Seq[Value],
) *Generator[Value] {
	generatorContext, cancel := context.WithCancel(parent)

	return &Generator[Value]{
		context: generatorContext,
		cancel:  cancel,
		source:  source,
	}
}

/*
Generate emits source values until exhaustion, cancellation, or a stopped
consumer.
*/
func (generator *Generator[Value]) Generate(
	iterators ...*iter.Seq[Value],
) <-chan Value {
	output := make(chan Value)
	source := iter.Seq[Value](nil)

	if generator != nil {
		source = generator.source
	}

	if len(iterators) > 0 && iterators[0] != nil {
		source = *iterators[0]
	}

	go func() {
		defer close(output)

		if generator == nil || source == nil {
			return
		}

		for value := range source {
			select {
			case <-generator.context.Done():
				return
			case output <- value:
			}
		}
	}()

	return output
}

/*
Close cancels generation.
*/
func (generator *Generator[Value]) Close() {
	if generator != nil && generator.cancel != nil {
		generator.cancel()
	}
}

/*
Error retains the former generator error-reporting surface. Iteration and
cancellation are not failures, so the adapter currently reports no error.
*/
func (generator *Generator[Value]) Error() string {
	return ""
}
