package cmd

import (
	"reflect"

	"github.com/theapemachine/symm/logic/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
gridNode feeds every signal measurement into the workspace's live numerical
grid. Grid updates, policy actions and inspection requests share this one
consumer. Separate disruptor stages can overlap different envelopes, so the
policy must not read the grid from a later stage.
*/
type gridNode struct {
	*learning.Grid
	prepare     []runtime.Node[*types.Envelope]
	publish     []runtime.Node[*types.Envelope]
	cognition   *cognition.Solver
	learner     *strategy.Agent
	err         error
	projections map[string]*[3]data.Measurement[float64]
	vectors     map[string][]string
	fieldNames  map[reflect.Type]map[string][]string
}

/* Step writes the canonical signal fields directly into the numerical grid. */
func (node *gridNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || node.err != nil {
		return envelope
	}

	for _, producer := range node.prepare {
		envelope = producer.Step(envelope)

		if failure, ok := producer.(runtime.ErrorNode); ok && failure.Error() != nil {
			node.err = failure.Error()
			return envelope
		}
	}

	// Keep inference and its numerical consumer in the same event turn.
	// A separate disruptor stage releases an entire batch at once; a long
	// cognition batch otherwise starves learning and operator inspection.
	if node.cognition != nil {
		envelope = node.cognition.Step(envelope)

		if node.err = node.cognition.Error(); node.err != nil {
			return envelope
		}
	}

	measurements := envelope.SignalMeasurements()
	inputs := [14]*data.Measurement[float64]{}
	copy(inputs[:], measurements[:])
	if err := node.project(envelope, inputs[11:]); err != nil {
		node.err = err
		return envelope
	}
	node.err = node.Grid.Step(inputs[:])

	if node.err != nil {
		return envelope
	}

	if node.learner != nil {
		envelope = node.learner.Step(envelope)

		if node.err = node.learner.Error(); node.err != nil {
			return envelope
		}
	}

	for _, publisher := range node.publish {
		envelope = publisher.Step(envelope)

		if failure, ok := publisher.(runtime.ErrorNode); ok && failure.Error() != nil {
			node.err = failure.Error()
			return envelope
		}
	}

	return envelope
}

/* Error exposes a failed update to the workload's existing error boundary. */
func (node *gridNode) Error() error {
	if node.err == nil && node.learner != nil {
		return node.learner.Error()
	}

	return node.err
}
