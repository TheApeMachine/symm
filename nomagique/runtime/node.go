package runtime

import (
	"github.com/theapemachine/symm/data"
)

/*
Node is the execution contract for an analytical stage in the workspace pipeline.
It receives an immutable prior WORM Measurement, allocates a fresh Measurement
from its owned arena, computes its owned facts, finalizes, and returns it.
The returned Measurement is WORM forever.
*/
type Node interface {
	Step(prior *data.Value) *data.Value
	Status() Stage
	Transition(Stage)
	Error(...error) error
	Name() string
}

/*
Consumer binds one Node to a Workspace sequence-safe ring.
It holds published WORM pointers for the ring capacity.
For each sequence:
 1. Invokes node.Step(prior).
 2. Enforces strict Source identity assertion.
 3. Stores the exact WORM pointer in published.
 4. Pushes the exact pointer to async Tees.

No StageInput. No reusable published measurement. No reset. No clone. No detached snapshot.
*/
type Consumer struct {
	node     Node
	mask     int64
	capacity int
	tees     []Tee
}

func NewConsumer(
	node Node,
	capacity int,
	mask int64,
	tees ...Tee,
) *Consumer {
	consumer := &Consumer{
		node:     node,
		mask:     mask,
		capacity: capacity,
		tees:     tees,
	}

	return consumer
}

func (consumer *Consumer) Step(value *data.Value) *data.Value {
	returned := consumer.node.Step(value)

	if len(consumer.tees) > 0 && returned != nil {
		for _, tee := range consumer.tees {
			if tee != nil {
				tee.Push(returned)
			}
		}
	}

	return value
}
