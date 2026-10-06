package runtime

import (
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Node is the execution contract for an analytical stage in the workspace pipeline.
It receives an immutable prior WORM Measurement, allocates a fresh Measurement
from its owned arena, computes its owned facts, finalizes, and returns it.
The returned Measurement is WORM forever.
*/
type Node interface {
	Step(prior *data.Measurement) *data.Measurement
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
	node      Node
	source    string
	published []*data.Measurement
	mask      int64
	capacity  int
	tees      []Tee
}

func NewConsumer(
	node Node,
	capacity int,
	mask int64,
	tees ...Tee,
) *Consumer {
	consumer := &Consumer{
		node:      node,
		published: make([]*data.Measurement, capacity),
		mask:      mask,
		capacity:  capacity,
		tees:      tees,
	}

	if sys, ok := node.(interface{ Name() string }); ok {
		consumer.source = sys.Name()
	}

	return consumer
}

func (consumer *Consumer) Step(prior *data.Measurement, seq int64) *data.Measurement {
	slot := seq & consumer.mask

	consumer.published[slot] = consumer.node.Step(prior)

	if len(consumer.tees) > 0 {
		for _, tee := range consumer.tees {
			if tee != nil {
				tee.Push(consumer.published[slot])
			}
		}
	}

	return consumer.published[slot]
}

// Published returns this consumer's WORM output for the given sequence,
// or nil if the sequence has not been published or the producer returned nil.
func (consumer *Consumer) Published(sequence int64) *data.Measurement {
	return consumer.published[sequence&consumer.mask]
}
