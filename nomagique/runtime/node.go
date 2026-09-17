package runtime

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Consumer delegates its input stream to an identifiable primitive.
It does not interpret the input or own routing, storage, or publication.
*/
type Consumer[T any] struct {
	*core.PrimitiveError
	node core.Identifiable[T]
}

func NewConsumer[T any](node core.Identifiable[T]) *Consumer[T] {
	consumer := &Consumer[T]{
		PrimitiveError: core.NewPrimitiveError(),
		node:           node,
	}

	return consumer
}

/*
Next returns the node's iterator unchanged.
*/
func (consumer *Consumer[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return consumer.node.Next(in)
}
