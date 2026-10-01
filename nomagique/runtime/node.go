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
Handle steps one node over every slot in [lower, upper].

Measurement payloads are Fork'd before Step (maps copied, Peers empty) so
concurrent HandlerGroup peers never share mutable maps and never clone the
peer forest. The owned result is Contribute'd as a Source-keyed peer only.
*/
func (consumer *Consumer[T]) Handle(lower, upper int64) {
	for sequence := lower; sequence <= upper; sequence++ {
		payload := consumer.buffer[sequence&consumer.mask]
		stepIn := payload

		var sharedMeas *data.Measurement[float64]
		if measurement, ok := any(payload).(*data.Measurement[float64]); ok && measurement != nil {
			sharedMeas = measurement
			stepIn = any(measurement.Fork()).(T)
		}

		result := consumer.node.Step(stepIn)

		if sharedMeas != nil {
			if owned, ok := any(result).(*data.Measurement[float64]); ok && owned != nil {
				sharedMeas.Contribute(owned)
			}
		}

		for _, tee := range consumer.tees {
			if tee != nil {
				if m, ok := any(result).(*data.Measurement[float64]); ok && m != nil {
					if m.SeqIdx <= 0 {
						m.SeqIdx = sequence + 1
					}
					tee.Push(m)
				}
			}
		}
	}
}
