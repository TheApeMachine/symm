package runtime

import (
	"github.com/theapemachine/symm/nomagique/data"
)

type Node[T any] interface {
	Step(T) T
}

/*
Consumer binds one Node to the Workload's register. At construction the node
identifies itself: Register() supplies the initial value if supported, an identify query
appends it and answers the slot, and the node is told its identity so the
values it produces name their own register slot.
*/
type Consumer[T any] struct {
	node   Node[T]
	buffer []T
	mask   int64
	tees   []Tee
}

func NewConsumer[T any](
	node Node[T], buffer []T, mask int64, tees ...Tee,
) *Consumer[T] {
	consumer := &Consumer[T]{
		node:   node,
		buffer: buffer,
		mask:   mask,
		tees:   tees,
	}

	return consumer
}

/*
Handle steps one node over every slot in [lower, upper]. Each invocation
reads the node's registered data back out of the register, passes it to Step,
and puts what Step returns back into the register under the node's slot.
Measurement sequences are one-based because zero means unstamped. Nil outputs
clear that sequence slot without replacing the node's last working state.
*/
func (consumer *Consumer[T]) Handle(lower, upper int64) {
	for sequence := lower; sequence <= upper; sequence++ {
		payload := consumer.buffer[sequence&consumer.mask]
		result := consumer.node.Step(payload)

		for _, tee := range consumer.tees {
			if tee != nil {
				// Convert to Measurement if it's a tee
				if m, ok := any(result).(*data.Measurement[float64]); ok {
					tee.Push(m)
				}
			}
		}
	}
}
