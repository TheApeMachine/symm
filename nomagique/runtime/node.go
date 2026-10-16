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
}

func NewConsumer(
	node Node,
	capacity int,
	mask int64,
) *Consumer {
	consumer := &Consumer{
		node:     node,
		mask:     mask,
		capacity: capacity,
	}

	return consumer
}

func (consumer *Consumer) Step(value *data.Value) *data.Value {
	consumer.node.Step(value)
	return value
}
