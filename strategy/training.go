package strategy

import (
	"container/ring"
	"context"
	"slices"
	"sync/atomic"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
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
	// mounted is the tape once the record has actually been read. It is only
	// ever published whole, so the ring never observes a half-built replay.
	mounted atomic.Pointer[replay]
	agents  int
}

/* replay is one mounted tape and the learners replaying it. */
type replay struct {
	pipeline  core.Primitive
	space     *grid.Space
	memories  []core.Primitive
	learners  []core.Primitive
	fragments int
	frames    uint64
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
	training := &Training{agents: agents}

	// Reading the record is I/O, so it never happens here and never on the
	// ring. Construction returns immediately and the tape is mounted once it
	// has been read; until then the learners have nothing to replay and say so.
	//
	// What it reads is bounded and ordered: the most recent runs first, and
	// only until the policy's declared number of episodes is mounted. Reading
	// the whole record instead contends with the capture writer, and capture
	// backpressure stalls the ring the instrument subscription is waiting on,
	// so the process never finishes starting.
	go training.mount(Archive(catalog, run, policy))

	return training
}

/* mount publishes a tape once it has been read, whole. */
func (training *Training) mount(fragments [][][]*data.Measurement[float64]) {
	training.mounted.Store(compose(fragments, training.agents))
}

/*
Archive is every confirmed move the record holds, across every run it holds one
for.

The run this process is starting has no moves in it yet — it has not observed
anything — so a learner pointed at its own run is handed an empty tape and
replays nothing. What there is to learn from is what was captured before, and
the current run is skipped rather than read: it is still being written, and its
last leg has not been confirmed.

How much is mounted is the discovery policy's to say. It already declares how
many episodes a set may hold, so that is the bound the walk stops at rather
than a second number invented here. Decoded readings stay resident while they
are mounted, so an unbounded walk is a memory claim as well as a read that
competes with whatever is writing the record.
*/
func Archive(
	catalog *tables.Catalog, current hindsight.RunID, policy hindsight.DiscoveryPolicy,
) [][][]*data.Measurement[float64] {
	if catalog == nil {
		return nil
	}
	runs, err := catalog.Runs(context.Background())

	if err != nil {
		errnie.Error(err)

		return nil
	}

	// Most recent first. What the instrument did lately describes it better
	// than what it did first, and the bound below stops the walk part way, so
	// which end it starts from decides what is learned from.
	slices.SortFunc(runs, func(left, right tables.RunRow) int {
		return right.StartedAt.Compare(left.StartedAt)
	})
	limit := policy.MaxEpisodesPerSet

	if limit <= 0 {
		limit = hindsight.DefaultDiscoveryPolicy().MaxEpisodesPerSet
	}
	fragments := make([][][]*data.Measurement[float64], 0, limit)

	for _, run := range runs {
		if run.ID == string(current) {
			continue
		}

		if len(fragments) >= limit {
			break
		}
		tape := hindsight.Query(
			hindsight.Excursions, catalog, hindsight.RunID(run.ID), policy,
		)
		found := tape.Measurements()

		if err := tape.Error(); err != nil {
			errnie.Error(err)

			continue
		}
		fragments = append(fragments, found...)
	}

	if len(fragments) > limit {
		fragments = fragments[:limit]
	}

	return fragments
}

/*
NewTrainingOver composes the same pipeline over fragments already in hand, so
the learning path can be exercised against a known tape rather than only
against whatever the archive happens to hold.
*/
func NewTrainingOver(
	fragments [][][]*data.Measurement[float64], agents int,
) *Training {
	training := &Training{agents: agents}
	training.mounted.Store(compose(fragments, agents))

	return training
}

/* compose builds one mounted tape and the learners that replay it. */
func compose(
	fragments [][][]*data.Measurement[float64], agents int,
) *replay {
	parent := ring.New(max(len(fragments), 1))

	for _, leg := range fragments {
		// Twice the slots, with an empty one after every frame. The store ends
		// a run on a slot it cannot read and resumes at the one after it, so a
		// frame is delivered on its own and the stage downstream advances once
		// per frame rather than being handed the whole leg at once.
		child := ring.New(len(leg) * 2)

		for _, measurement := range leg {
			child.Value = core.From(measurement)
			child = child.Next().Next()
		}

		parent = parent.Next()
		parent.Value = child
	}
	space := grid.NewSpace()
	memories := make([]core.Primitive, 0, max(agents, 1))
	learners := make([]core.Primitive, 0, max(agents, 1))

	for range max(agents, 1) {
		memory := associative.NewMemory()
		memories = append(memories, memory)
		learners = append(learners, associative.NewAgent(memory))
	}

	return &replay{
		space:     space,
		memories:  memories,
		learners:  learners,
		fragments: len(fragments),
		pipeline: nomagique.Number(
			store.NewRing(core.From(parent)),
			space,
			transport.NewFan(
				transport.NewPipe(),
				transport.NewIO(learners...),
			),
		),
	}
}

/*
Step drains one delivery run — one tape fragment — and hands the envelope back
unchanged. The envelope is the runtime's clock here, not the evidence.
*/
func (training *Training) Step(envelope *types.Envelope) *types.Envelope {
	held := training.mounted.Load()

	if held == nil {
		// The record has not been read yet. Nothing is replayed and nothing is
		// claimed: an unmounted tape is not an empty one.
		if envelope != nil {
			envelope.Learning = training
		}

		return envelope
	}

	for value := held.pipeline.Next(nil); value != nil; value = held.pipeline.Next(nil) {
		held.frames++
	}

	// The envelope carries the owner, not a reading taken from it. Building one
	// here would pay for every learner's whole memory on every step, and the
	// socket keeps roughly none of them.
	if envelope != nil {
		envelope.Learning = training
	}

	return envelope
}

/* Space is the grid the fragments are replayed through. */
func (training *Training) Space() *grid.Space {
	if held := training.mounted.Load(); held != nil {
		return held.space
	}

	return nil
}

/* Learners are the agents sharing the tape, each with its own memory. */
func (training *Training) Learners() []core.Primitive {
	if held := training.mounted.Load(); held != nil {
		return held.learners
	}

	return nil
}

/* Memories are what each agent has learned, readable without disturbing it. */
func (training *Training) Memories() []core.Primitive {
	if held := training.mounted.Load(); held != nil {
		return held.memories
	}

	return nil
}

/* Error exposes the composition's failure through the runtime node protocol. */
func (training *Training) Error() error {
	if held := training.mounted.Load(); held != nil {
		return held.pipeline.Error()
	}

	return nil
}
