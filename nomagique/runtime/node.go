package runtime

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
Node is the execution contract for an analytical stage in the workspace pipeline.
It receives an immutable prior WORM Measurement, allocates a fresh Measurement
from its owned arena, computes its owned facts, finalizes, and returns it.
The returned Measurement is WORM forever.
*/
type Node interface {
	Step(prior *data.Measurement[float64]) *data.Measurement[float64]
}

/*
ArenaAware is implemented by nodes that own an ArenaOwner.
*/
type ArenaAware interface {
	Arena() *data.ArenaOwner
}

/*
Consumer binds one Node to a Workspace sequence-safe ring.
It holds published WORM pointers for the ring capacity.
For each sequence:
 1. Advances the node's ArenaOwner to rotate generations at ring boundaries.
 2. Invokes node.Step(prior).
 3. Enforces strict Source identity assertion.
 4. Stores the exact WORM pointer in published.
 5. Pushes the exact pointer with its ArenaGeneration token to async Tees.

No StageInput. No reusable published measurement. No reset. No clone. No detached snapshot.
*/
type Consumer struct {
	node      Node
	source    string
	arena     *data.ArenaOwner
	published []*data.Measurement[float64]
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
		published: make([]*data.Measurement[float64], capacity),
		mask:      mask,
		capacity:  capacity,
		tees:      tees,
	}

	if sys, ok := node.(interface{ Name() string }); ok {
		consumer.source = sys.Name()
	}

	if aa, ok := node.(ArenaAware); ok {
		consumer.arena = aa.Arena()
	}

	if consumer.arena == nil {
		consumer.arena = data.NewArenaOwner(capacity)
	}

	if consumer.arena != nil {
		consumer.arena.SetWindow(capacity)
	}

	return consumer
}

func (consumer *Consumer) Step(prior *data.Measurement[float64], seq int64) *data.Measurement[float64] {
	slot := seq & consumer.mask

	if consumer.arena != nil {
		consumer.arena.Advance(seq)
	}

	result := consumer.node.Step(prior)

	if result != nil {
		if result.Epoch == 0 && prior != nil {
			result.Epoch = prior.Epoch
		}
		if result.Tick == 0 && prior != nil {
			result.Tick = prior.Tick
		}
		if result.Source == "" {
			panic("source identity missing: node returned measurement with empty Source")
		}
		if consumer.source != "" && result.Source != consumer.source {
			panic(fmt.Sprintf("source mismatch: declared %q but got %q", consumer.source, result.Source))
		}
	}

	consumer.published[slot] = result

	if result != nil && len(consumer.tees) > 0 {
		var gen *data.ArenaGeneration
		if consumer.arena != nil {
			gen = consumer.arena.CurrentGeneration()
		}

		pub := data.Publication{
			Measurement: result,
			Generation:  gen,
		}

		for _, tee := range consumer.tees {
			if tee != nil {
				tee.Push(pub)
			}
		}
	}

	return result
}

// Published returns this consumer's WORM output for the given sequence,
// or nil if the sequence has not been published or the producer returned nil.
func (consumer *Consumer) Published(sequence int64) *data.Measurement[float64] {
	return consumer.published[sequence&consumer.mask]
}

// Name returns this consumer's producer identity.
func (consumer *Consumer) Name() string {
	return consumer.source
}

// Arena returns this consumer's ArenaOwner.
func (consumer *Consumer) Arena() *data.ArenaOwner {
	return consumer.arena
}

// Node returns the underlying Node.
func (consumer *Consumer) Node() Node {
	return consumer.node
}
