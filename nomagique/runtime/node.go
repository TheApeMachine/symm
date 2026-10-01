package runtime

import (
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Node is the execution contract for an analytical stage in the workspace pipeline.
It reads immutable prior workspace evidence via input (*StageInput) and writes its
own produced metrics directly into its owned output measurement (*data.Measurement[float64]).
After Step returns, output is published and becomes READ-ONLY (WORM).
*/
type Node interface {
	Step(input *StageInput, output *data.Measurement[float64]) *data.Measurement[float64]
}

/*
Registrar is an optional interface implemented by nodes that declare their
initial output measurement schema and source identity.
*/
type Registrar interface {
	Register() *data.Measurement[float64]
}

/*
Sourcer is an optional interface implemented by nodes to report their source name.
*/
type Sourcer interface {
	Source() string
}

/*
Consumer binds one Node to a Workspace sequence-safe ring.
It preallocates a bounded ring of output Measurement slots sized to the Workspace
ring capacity: O(producers × workspace ring capacity × producer schema).

At safe slot reuse, Consumer resets only its own reusable output slot, preserving
declared metric schema and map capacity. It passes read-only StageInput and the
owned output slot to the node's Step, and publishes at most ONE detached immutable
heap snapshot shared by StoreTee and UITee.
*/
type Consumer struct {
	node        Node
	source      string
	slots       []*data.Measurement[float64]
	published   []*data.Measurement[float64]
	stageInputs []*StageInput
	mask        int64
	tees        []Tee
}

func NewConsumer(
	node Node,
	capacity int,
	mask int64,
	stageInputs []*StageInput,
	tees ...Tee,
) *Consumer {
	consumer := &Consumer{
		node:        node,
		slots:       make([]*data.Measurement[float64], capacity),
		published:   make([]*data.Measurement[float64], capacity),
		stageInputs: stageInputs,
		mask:        mask,
		tees:        tees,
	}

	var template *data.Measurement[float64]
	if reg, ok := node.(Registrar); ok {
		template = reg.Register()
	}

	source := ""
	if template != nil && template.Source != "" {
		source = template.Source
	} else if src, ok := node.(Sourcer); ok {
		source = src.Source()
	} else if sys, ok := node.(interface{ Name() string }); ok {
		source = sys.Name()
	}
	consumer.source = source

	for i := 0; i < capacity; i++ {
		if template != nil {
			consumer.slots[i] = template.Clone()
		} else {
			consumer.slots[i] = data.NewMeasurement[float64](source, nil)
		}
	}

	return consumer
}

/*
Handle steps one node over every slot in [lower, upper].

For each sequence:
 1. Build the read-only StageInput from completed prior stages.
 2. Reset this producer's preallocated output slot (preserving schema/capacity).
 3. Stamp the output with sequence, time, symbol, and source identity.
 4. Pass read-only input and owned output to the node's Step.
 5. After Step returns, the output is WORM (published, read-only).
 6. Create exactly ONE DetachedSnapshot for async Tee queues.
 7. Push the shared snapshot to all Tees immediately.

No Fork. No Contribute. No shared mutable Measurement. No Peers forest growth.
*/
func (consumer *Consumer) Handle(lower, upper int64) {
	for sequence := lower; sequence <= upper; sequence++ {
		slot := sequence & consumer.mask
		input := consumer.stageInputs[slot]
		outputSlot := consumer.slots[slot]

		outputSlot.ResetSlot()
		outputSlot.SeqIdx = sequence + 1
		outputSlot.Timestamp = input.At().UnixNano()
		outputSlot.At = input.At()
		outputSlot.From = input.From()
		outputSlot.Label = input.Symbol()
		if consumer.source != "" {
			outputSlot.Source = consumer.source
		}

		result := consumer.node.Step(input, outputSlot)

		if result == nil {
			consumer.published[slot] = nil
			continue
		}

		consumer.published[slot] = result

		var snapshot *data.Measurement[float64]
		for _, tee := range consumer.tees {
			if tee != nil {
				if snapshot == nil {
					snapshot = result.DetachedSnapshot()
				}
				tee.Push(snapshot)
			}
		}
	}
}

// Published returns this consumer's WORM output for the given sequence,
// or nil if the sequence has not been published or the producer returned nil.
func (consumer *Consumer) Published(sequence int64) *data.Measurement[float64] {
	return consumer.published[sequence&consumer.mask]
}

// Source returns this consumer's producer identity.
func (consumer *Consumer) Source() string {
	return consumer.source
}
