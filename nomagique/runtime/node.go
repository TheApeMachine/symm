package runtime

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Consumer delegates its input stream to a primitive.
It does not interpret the input or own routing, storage, or publication.
*/
type Consumer struct {
	*core.PrimitiveError
	node core.Primitive
}

func NewConsumer(node core.Primitive) *Consumer {
	consumer := &Consumer{
		PrimitiveError: core.NewPrimitiveError(),
		node:           node,
	}

	return consumer
}

/*
Next returns the node's iterator unchanged.
*/
func (consumer *Consumer) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return consumer.node.Next(in)
}
