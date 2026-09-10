package strategy

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
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
	// frames is the cumulative number of measurements drained across every
	// published tape. It lives here rather than on replay so refreshing the
	// tape never resets the dashboard's observation counter.
	frames atomic.Uint64
	agents int
}

/* replay is one mounted tape and the learners replaying it. */
type replay struct {
	space     *grid.Space
	memories  []*store.Retained[*iradix.Tree[[]byte]]
	learners  []*associative.Agent
	fragments int
	fragment  int
	frame     int
	err       error

	// tape is the mounted fragments themselves, kept so the dashboard can
	// draw each learner's lane without disturbing the live pipeline.
	tape [][][]*data.Measurement[float64]
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
	// ring. Construction returns immediately with an empty tape mounted, and
	// the loader atomically replaces it as each run it walks yields more
	// confirmed moves. An unmounted tape would keep the dashboard on
	// "reading the record" until the whole archive had been walked; publishing
	// per run instead lets it fill in as the walk proceeds.
	training.publish(nil)

	if catalog == nil {
		return training
	}

	go training.load(catalog, run, policy)

	return training
}

/* publish atomically swaps the mounted tape for the one the walk has reached. */
func (training *Training) publish(fragments [][][]*data.Measurement[float64]) {
	previous := training.mounted.Load()
	training.mounted.Store(composeOver(previous, fragments, training.agents))
}

/*
load walks the record newest run first and mounts what each one holds, one run
at a time.

A run is the smallest read that can hold a confirmed move, so it is also the
smallest step that can add anything to the tape: a move qualifies on its own
geometry and is confirmed only once price retraced away from the extremum it
reached, and a discovery needs the policy's minimum observations for a symbol
before it will look at all. Publishing per run rather than per page is what
lets the dashboard fill in as the walk proceeds without ever mounting a tape
built from a fraction of a run.

The run this process is starting is skipped. It has not observed anything yet,
it is still being written, and its last leg has not been confirmed, so a
learner pointed at it is handed an empty tape and replays nothing. What there
is to learn from is what was captured before.

How much is walked is the rehearsal budget's to say: runs are opened newest
first until that many observations are resident. A run is always read whole —
stopping mid-run would end a tape at the budget rather than at the record — so
the budget decides how many runs are opened, never how much of one is read.
*/
func (training *Training) load(
	catalog *tables.Catalog, current hindsight.RunID, policy hindsight.DiscoveryPolicy,
) {
	limit := policy.MaxEpisodesPerSet

	if limit <= 0 {
		limit = hindsight.DefaultDiscoveryPolicy().MaxEpisodesPerSet
	}
	budget := viper.GetInt("hindsight.rehearsal.observation_budget")

	if budget <= 0 {
		fallback := hindsight.DefaultDiscoveryPolicy()
		budget = fallback.MinObservations * fallback.MaxEpisodesPerSet
	}
	ctx := context.Background()
	runs, err := catalog.Runs(ctx)

	if err != nil {
		errnie.Error(err)

		return
	}

	// Most recent first. What the instrument did lately describes it better
	// than what it did first, and the budget stops the walk part way, so which
	// end it starts from decides what is learned from.
	slices.SortFunc(runs, func(left, right tables.RunRow) int {
		return right.StartedAt.Compare(left.StartedAt)
	})
	fragments := make([][][]*data.Measurement[float64], 0, limit)
	resident := 0

	for _, run := range runs {
		if run.ID == string(current) {
			continue
		}

		if resident >= budget || len(fragments) >= limit {
			break
		}

		// Read whole and in capture order. A bounded read is not an option
		// here: Iceberg plans files in its own order, so a row limit returns
		// an arbitrary slice of the run rather than its next rows, and a
		// cursor taken from one skips everything the planner did not reach.
		observations, _, err := hindsight.ReadObservations(
			ctx, catalog, hindsight.RunID(run.ID), 0,
		)

		if err != nil {
			errnie.Error(err)

			continue
		}
		resident += len(observations)
		tape := hindsight.Query(
			hindsight.Excursions, catalog, hindsight.RunID(run.ID), policy,
		)
		found := tape.MeasurementsFrom(observations)

		if err := tape.Error(); err != nil {
			errnie.Error(err)

			continue
		}

		// A run the record kept no confirmed move for adds nothing, and
		// republishing an unchanged tape would rebuild the ring the learners
		// are already replaying for no reason.
		if len(found) == 0 {
			continue
		}
		fragments = append(fragments, found...)

		if len(fragments) > limit {
			fragments = fragments[:limit]
		}

		training.publish(fragments)
	}
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
	training.publish(fragments)

	return training
}

/*
composeOver builds the tape for the latest fragments while preserving the
learners the previous tape already trained. The grid and every agent's memory
are state: recreating them on each page would discard what the earlier pages
taught, so a refreshed tape reuses them and only replaces the ring they are
being shown.
*/
func composeOver(
	previous *replay, fragments [][][]*data.Measurement[float64], agents int,
) *replay {
	if previous == nil {
		space := grid.NewSpace()
		memories := make([]*store.Retained[*iradix.Tree[[]byte]], 0, max(agents, 1))
		learners := make([]*associative.Agent, 0, max(agents, 1))

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
			tape:      fragments,
		}
	}

	return &replay{
		space:     previous.space,
		memories:  previous.memories,
		learners:  previous.learners,
		fragments: len(fragments),
		tape:      fragments,
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

	// An empty tape is not replayed. The fan hands every learner its (empty)
	// delivery run regardless, so draining one would step all of them against
	// nothing and count each step as a frame — the dashboard would report
	// observations it never held.
	if held.fragments == 0 {
		if envelope != nil {
			envelope.Learning = training
		}

		return envelope
	}

	if err := held.step(); err != nil {
		held.err = err
	}

	if held.err == nil {
		training.frames.Add(1)
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
func (training *Training) Learners() []*associative.Agent {
	if held := training.mounted.Load(); held != nil {
		return held.learners
	}

	return nil
}

/* Memories are what each agent has learned, readable without disturbing it. */
func (training *Training) Memories() []*store.Retained[*iradix.Tree[[]byte]] {
	if held := training.mounted.Load(); held != nil {
		return held.memories
	}

	return nil
}

/* Error exposes the composition's failure through the runtime node protocol. */
func (training *Training) Error() error {
	if held := training.mounted.Load(); held != nil {
		return errors.Join(held.err, held.space.Error())
	}

	return nil
}

func (held *replay) step() error {
	if len(held.tape) == 0 {
		return nil
	}

	measurements := held.tape[held.fragment][held.frame]
	held.frame++

	if held.frame >= len(held.tape[held.fragment]) {
		held.frame = 0
		held.fragment = (held.fragment + 1) % len(held.tape)
	}

	impulse, err := transport.Evaluate(held.space, transport.Values(measurements))
	if err != nil {
		return err
	}

	for _, learner := range held.learners {
		if _, err := learner.Learn(impulse); err != nil {
			return err
		}
	}

	return nil
}
