package strategy

import (
	"context"
	"errors"
	"iter"
	"sync"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
Training is one contained learning pipeline. Every stage is
Primitive[*types.Envelope, *types.Envelope], which is the value Step already
holds, so Number can thread them as one run.
*/
type Training struct {
	*runtime.System
	mu       sync.Mutex
	tape     <-chan [][]*data.Measurement[float64]
	query    *store.QueryOp[*types.Envelope]
	frames   *frames
	space    *space
	judge    *judge
	agent    *agent
	pipeline func() iter.Seq[core.Primitive[any, any]]
	legs     [][][]*data.Measurement[float64]
	seen     atomic.Uint64
}

/* replay is the dashboard's reading of this pipeline. */
type replay struct {
	space     *grid.Space
	memories  []*store.Retained[*iradix.Tree[[]byte]]
	learners  []*associative.Agent
	fragments int
	tape      [][][]*data.Measurement[float64]
	loading   bool
}

/*
NewTraining builds a learning pipeline over a tape that arrives as it is read.

The record is on the far side of an object store, and walking it takes long
enough that doing it here would hold the boot up until it finished. So the tape
is a channel rather than a slice: the pipeline is composed immediately over an
empty ring, and each fragment the reader recovers is played as soon as it
lands. An empty ring is a run with nothing in it, which is what a learner that
has not been shown anything yet honestly is.
*/
func NewTraining(
	ctx context.Context,
	tape <-chan [][]*data.Measurement[float64],
) *Training {
	memory := associative.NewMemory()
	training := &Training{
		tape:   tape,
		query:  store.Query[*types.Envelope](),
		frames: newFrames(),
		space:  newSpace(),
		judge:  newJudge(memory),
		agent:  newAgent(memory),
		System: runtime.NewSystem(ctx, "training"),
	}
	training.pipeline = nomagique.Number(
		training.query,
		training.frames,
		training.space,
		training.judge,
		training.agent,
	)

	return training
}

/*
Step is one delivery of the composition.

The envelope is the run. Query holds it, Number threads it through every
Envelope-typed stage, and what comes back is the same envelope with whatever
those stages wrote.
*/
func (training *Training) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || training.pipeline == nil {
		return envelope
	}

	training.mount()
	training.query.Carrier(envelope)

	for range training.pipeline() {
	}

	envelope.Learning = training
	training.seen.Add(1)

	return envelope
}

/*
mount takes up whatever the reader has recovered since the last step.

It never waits. The envelope that asked for this step is a market frame the
runtime is holding, so blocking here to see whether more tape is coming would
stall ingress on a read of the record. What has landed is played; what has not
is played on a later step.
*/
func (training *Training) mount() {
	for {
		select {
		case fragment, open := <-training.tape:
			if !open {
				training.mu.Lock()
				training.tape = nil
				training.mu.Unlock()

				return
			}

			training.mu.Lock()
			training.legs = append(training.legs, fragment)
			training.mu.Unlock()

			for _, measurements := range fragment {
				child := store.NewRing[*types.Envelope]()
				child.Write(&types.Envelope{Observations: measurements})
				training.frames.ring.Write(child)
			}
		default:
			return
		}
	}
}

func (training *Training) snapshot() *replay {
	training.mu.Lock()
	defer training.mu.Unlock()

	return &replay{
		space:     training.space.grid,
		memories:  []*store.Retained[*iradix.Tree[[]byte]]{training.agent.memory},
		learners:  []*associative.Agent{training.agent.inner},
		fragments: len(training.legs),
		tape:      training.legs,
		loading:   training.tape != nil,
	}
}

/* Error joins every stage's failure for the runtime node protocol. */
func (training *Training) Error() error {
	return errors.Join(
		training.frames.Error(),
		training.space.Error(),
		training.space.grid.Error(),
		training.judge.Error(),
		training.judge.inner.Error(),
		training.agent.Error(),
		training.agent.inner.Error(),
	)
}

/*
frames plays one child of the tape ring onto the envelope that arrived.

The child is itself a ring of envelopes, each carrying one numerical frame.
One Next is one child, which is the delivery boundary the parent ring owns.
*/
type frames struct {
	core.Base[*types.Envelope, *types.Envelope]
	ring *store.Ring[*types.Envelope]
}

func newFrames() *frames {
	return &frames{ring: store.NewRing[*types.Envelope]()}
}

func (op *frames) Next(
	in iter.Seq[core.Primitive[*types.Envelope, *types.Envelope]],
) iter.Seq[core.Primitive[*types.Envelope, *types.Envelope]] {
	return func(yield func(core.Primitive[*types.Envelope, *types.Envelope]) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			envelope := arriving.Read()

			if envelope == nil {
				return
			}

			played := false

			for frame := range op.ring.Next(nil) {
				played = true
				held := frame.Read()

				if held != nil {
					envelope.Observations = held.Observations
				}

				if !yield(op.Carrier(envelope)) {
					return
				}
			}

			if !played {
				if !yield(op.Carrier(envelope)) {
					return
				}
			}
		}
	}
}

/*
space steps the grid from the envelope's observations and writes the impulse
back onto it.
*/
type space struct {
	core.Base[*types.Envelope, *types.Envelope]
	grid *grid.Space
}

func newSpace() *space {
	return &space{grid: grid.NewSpace()}
}

func (op *space) Next(
	in iter.Seq[core.Primitive[*types.Envelope, *types.Envelope]],
) iter.Seq[core.Primitive[*types.Envelope, *types.Envelope]] {
	return func(yield func(core.Primitive[*types.Envelope, *types.Envelope]) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			envelope := arriving.Read()

			if envelope == nil {
				return
			}

			if len(envelope.Observations) == 0 {
				if !yield(op.Carrier(envelope)) {
					return
				}

				continue
			}

			impulse, err := transport.Evaluate(
				op.grid, transport.Values(envelope.Observations),
			)

			if err != nil {
				op.Error(err)

				return
			}

			envelope.Impulses = append(envelope.Impulses[:0], impulse)

			if !yield(op.Carrier(envelope)) {
				return
			}
		}
	}
}

/*
judge grades the envelope's impulse against the agent's memory before the
agent writes.
*/
type judge struct {
	core.Base[*types.Envelope, *types.Envelope]
	inner *associative.EvaluatorOp
}

func newJudge(memory *store.Retained[*iradix.Tree[[]byte]]) *judge {
	return &judge{inner: associative.Evaluator(memory)}
}

func (op *judge) Next(
	in iter.Seq[core.Primitive[*types.Envelope, *types.Envelope]],
) iter.Seq[core.Primitive[*types.Envelope, *types.Envelope]] {
	return func(yield func(core.Primitive[*types.Envelope, *types.Envelope]) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			envelope := arriving.Read()

			if envelope == nil {
				return
			}

			if len(envelope.Impulses) == 0 {
				if !yield(op.Carrier(envelope)) {
					return
				}

				continue
			}

			impulse := envelope.Impulses[len(envelope.Impulses)-1]
			assoc, err := transport.Evaluate(op.inner, transport.Values(impulse))

			if err != nil {
				op.Error(err)

				return
			}

			impulse.Graded = assoc.Graded
			impulse.Grade = assoc.Feedback
			envelope.Impulses[len(envelope.Impulses)-1] = impulse

			if !yield(op.Carrier(envelope)) {
				return
			}
		}
	}
}

/*
agent learns the envelope's impulse into its own memory.
*/
type agent struct {
	core.Base[*types.Envelope, *types.Envelope]
	inner  *associative.Agent
	memory *store.Retained[*iradix.Tree[[]byte]]
}

func newAgent(memory *store.Retained[*iradix.Tree[[]byte]]) *agent {
	return &agent{inner: associative.NewAgent(memory), memory: memory}
}

func (op *agent) Next(
	in iter.Seq[core.Primitive[*types.Envelope, *types.Envelope]],
) iter.Seq[core.Primitive[*types.Envelope, *types.Envelope]] {
	return func(yield func(core.Primitive[*types.Envelope, *types.Envelope]) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			envelope := arriving.Read()

			if envelope == nil {
				return
			}

			if len(envelope.Impulses) == 0 {
				if !yield(op.Carrier(envelope)) {
					return
				}

				continue
			}

			if _, err := op.inner.Learn(envelope.Impulses[len(envelope.Impulses)-1]); err != nil {
				op.Error(err)

				return
			}

			if !yield(op.Carrier(envelope)) {
				return
			}
		}
	}
}
