package strategy

import (
	"container/ring"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
Training replays recorded tape fragments through the grid and into the agents
that learn their precursors.

One delivery run is one fragment: the store closes a run when a fragment ends,
and the composition carries that boundary all the way down, so an agent is
never handed the value after a fragment's last as if it were its successor.
*/
type Training struct {
	pipeline core.Primitive
}

/*
NewTraining composes the store over recorded fragments into the grid, fanned
out across agents.

fragments is the retrieved excursion data — each one a run of the readings the
running binary held across a confirmed move. They are presented item by item,
with the algebra's own run boundary between fragments, which is what stops the
store iterating forever.

agents is how many learners share the work. Each owns its own cognition engine,
because what one learned about a precursor is its own memory and not a register
the others write through.
*/
func NewTraining(
	catalog *tables.Catalog,
	run hindsight.RunID,
	policy hindsight.DiscoveryPolicy,
	agents int,
) *Training {
	tape := hindsight.Query(hindsight.Excursions, catalog, run, policy)
	measurements := tape.Measurements()

	parent := ring.New(len(measurements))

	for _, leg := range measurements {
		child := ring.New(len(leg))

		for _, measurement := range leg {
			child.Value = core.From(measurement)
			child = child.Next()
		}

		parent = parent.Next()
		parent.Value = child
	}

	return &Training{
		pipeline: nomagique.Number(
			store.NewRing(core.From(parent)),
			grid.NewSpace(),
			transport.NewFan(
				transport.NewPipe(),
				transport.NewIO(
					associative.NewAgent(),
					associative.NewAgent(),
					associative.NewAgent(),
					associative.NewAgent(),
					associative.NewAgent(),
					associative.NewAgent(),
					associative.NewAgent(),
				),
			),
		),
	}
}

/*
Step drains one delivery run — one tape fragment — and hands the envelope back
unchanged. The envelope is the runtime's clock here, not the evidence.
*/
func (training *Training) Step(envelope *types.Envelope) *types.Envelope {
	for value := training.pipeline.Next(nil); value != nil; value = training.pipeline.Next(nil) {
	}

	return envelope
}

/* Error exposes the composition's failure through the runtime node protocol. */
func (training *Training) Error() error { return training.pipeline.Error() }
