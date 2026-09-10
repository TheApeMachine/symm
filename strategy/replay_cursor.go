package strategy

import (
	"container/ring"
	"context"
	"maps"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

// replayCursor owns one learner's ring and in-progress exercise. One Step
// advances one captured observation or resolves one completed decision.
// Pending additions enter the circular list only between complete exercises.
type replayCursor struct {
	rehearsal *Rehearsal
	ctx       context.Context
	mutex     sync.Mutex
	pending   []fragment
	fragments *ring.Ring
	learned   *cognition.Engine
	tape      fragment
	mounted   bool
	space     *grid.Space
	session   *practice
	member    *agent.Agent[Action]
	track     wire.LearningTrackT
	index     int
}

func (cursor *replayCursor) Step(envelope *types.Envelope) *types.Envelope {
	if cursor.ctx.Err() != nil || cursor.rehearsal.Error() != nil {
		return envelope
	}

	if !cursor.mounted {
		cursor.link()

		if cursor.fragments == nil {
			return envelope
		}

		if err := cursor.begin(); err != nil {
			cursor.rehearsal.fail(err)
			return envelope
		}
	}
	var err error

	if cursor.index < len(cursor.tape.observations) {
		err = cursor.advance()
	}

	if err == nil && cursor.index == len(cursor.tape.observations) {
		cursor.finish()
	}

	if err != nil {
		cursor.rehearsal.fail(err)
	}
	return envelope
}

func (cursor *replayCursor) link() {
	cursor.mutex.Lock()
	defer cursor.mutex.Unlock()

	if len(cursor.pending) == 0 {
		return
	}
	addition := ring.New(len(cursor.pending))
	for _, tape := range cursor.pending {
		addition.Value = tape
		addition = addition.Next()
	}

	if cursor.fragments != nil {
		cursor.fragments.Link(addition)
	}

	if cursor.fragments == nil {
		cursor.fragments = addition
	}
	cursor.pending = nil
}

func (cursor *replayCursor) begin() error {
	if cursor.member != nil {
		if err := cursor.member.Close(); err != nil {
			return errnie.Error(err)
		}
	}
	cursor.tape = cursor.fragments.Value.(fragment)
	cursor.mounted = true
	if cursor.space == nil {
		cursor.space = grid.NewSpace()
	}
	cursor.space.Reset()
	symbol := ""

	if len(cursor.tape.observations) > 0 {
		symbol = cursor.tape.observations[0].Symbol
	}
	cursor.session = &practice{symbol: symbol, entry: cursor.tape.entry, exit: cursor.tape.exit}
	member, err := agent.New(cursor.ctx, cursor.session, cursor.learned, true)
	cursor.member, cursor.index = member, 0
	cursor.mount()
	return errnie.Error(err)
}

// trackBudget bounds how many of a fragment's observations are delivered to a
// viewer. It is a delivery budget only: index, length and every mark position
// stay in the fragment's own observation coordinates.
const trackBudget = 256

/*
mount publishes the fragment this worker just picked up: the captured series
itself, read through the same market coordinate episode discovery selected.
An observation that coordinate is undefined at is delivered as absent.
*/
func (cursor *replayCursor) mount() {
	observations := cursor.tape.observations
	stride := 1 + len(observations)/trackBudget
	steps := make([]*wire.LearningStepT, 0, len(observations)/stride+1)

	for index := 0; index < len(observations); index += stride {
		value, defined := observations[index].Value(cursor.rehearsal.policy.Coordinate)
		steps = append(steps, &wire.LearningStepT{
			AtNs: observations[index].At().UnixNano(), Value: value, Defined: defined,
		})
	}
	symbol := ""

	if len(observations) > 0 {
		symbol = observations[0].Symbol
	}
	queued := 0

	if cursor.fragments != nil {
		queued = cursor.fragments.Len()
	}
	cursor.mutex.Lock()
	defer cursor.mutex.Unlock()
	cursor.track = wire.LearningTrackT{
		Symbol: symbol, Length: int32(len(observations)), Stride: int32(stride),
		Queued: int32(queued), Steps: steps,
		Entry: int32(cursor.tape.entry), Exit: int32(cursor.tape.exit),
		Opportunity: cursor.tape.opportunity,
	}
}

/*
wire copies this worker's mounted track. steps are fixed for the life of the
fragment and are shared; marks are copied because a grade lands on an already
published mark.
*/
func (cursor *replayCursor) wire(id int32) *wire.LearningTrackT {
	cursor.mutex.Lock()
	defer cursor.mutex.Unlock()
	track := cursor.track
	track.Id = id
	track.Marks = make([]*wire.LearningMarkT, len(cursor.track.Marks))

	for index, mark := range cursor.track.Marks {
		copied := *mark
		track.Marks[index] = &copied
	}

	return &track
}

/*
advance shows the worker one observation: the grid takes the captured
measurements, and if they complete an impulse the worker is asked what it makes
of the system state. The answer is judged against the tape's own geometry
straight away, because the moment it was asked about already happened.
*/
func (cursor *replayCursor) advance() error {
	index := cursor.index
	cursor.index++
	cursor.mutex.Lock()
	cursor.track.Index = int32(cursor.index)
	cursor.mutex.Unlock()
	observation := cursor.tape.observations[index]
	// Objective marks use this worker's actual processing clock. Captured
	// venue and receive times can both regress across ordered input frames;
	// they remain untouched on observations and impulses for causal context.
	cursor.session.index, cursor.session.at = index, time.Now()

	measurements := make([]*data.Measurement[float64], 0, len(observation.Measurements))
	from := observation.ReceivedAt
	for _, captured := range observation.Measurements {
		if captured.Label != observation.Symbol {
			continue
		}
		measurement := *captured
		measurement.Metrics = maps.Clone(captured.Metrics)
		measurements = append(measurements, &measurement)

		if !measurement.From.IsZero() && measurement.From.Before(from) {
			from = measurement.From
		}
	}

	// An observation carrying no captured precursor cannot activate a row or
	// reuse a previous impulse. The worker is not asked about it.
	if len(measurements) == 0 {
		cursor.member.Transition(runtime.WAITING)
		cursor.unsupported()
		return nil
	}

	if err := cursor.space.Step(measurements); err != nil {
		return errnie.Error(err)
	}
	impulse, err := cursor.space.Impulse(observation.Symbol, observation.ReceivedAt, from)

	if err != nil {
		return errnie.Error(err)
	}

	// Formation survives tape changes. Only observation-local baselines reset;
	// a quiet tape cannot send a formed grid back through calibration.
	if !impulse.Ready {
		cursor.member.Transition(runtime.WAITING)
		cursor.rehearsal.mutex.Lock()
		cursor.rehearsal.progress.Warming++
		cursor.rehearsal.mutex.Unlock()
		return nil
	}
	before := cursor.member.Decisions
	holding := cursor.session.holding
	cursor.member.Step(impulse)

	if err := cursor.member.Error(); err != nil {
		return errnie.Error(err)
	}

	if cursor.member.Decisions == before {
		return nil
	}
	return cursor.grade(index, holding, cursor.member.Last)
}

/*
grade judges one call and teaches it. The worker is told how well it named the
moment, and the shared policy absorbs the same number against the exact
precursor the worker was looking at when it answered.
*/
func (cursor *replayCursor) grade(
	index int, holding bool, decision *agent.Decision[Action],
) error {
	// judge reads the position state the call was made from, not the one it
	// produced: an entry is judged as an entry, never as a fresh hold.
	session := practice{holding: holding}
	value, verdict := session.judge(cursor.tape, index, decision.Action)

	if _, err := cursor.member.Resolve(decision.ID, value); err != nil {
		return errnie.Error(err)
	}

	if err := cursor.rehearsal.learner.Learn(decision.Label, cursor.space.Columns,
		decision.Context, decision.Action, value, decision.Authority); err != nil {
		return errnie.Error(err)
	}
	cursor.mark(index, decision, value, verdict)
	cursor.rehearsal.mutex.Lock()
	defer cursor.rehearsal.mutex.Unlock()
	cursor.rehearsal.progress.Decisions++
	cursor.rehearsal.progress.Trained++
	cursor.rehearsal.progress.LastSymbol = decision.Label
	cursor.rehearsal.progress.LastAction = decision.Action.Kind
	cursor.rehearsal.progress.LastReturn = value
	cursor.rehearsal.progress.LastFailure = verdict
	return nil
}

/* finish rotates to the next tape; a completed pass teaches nothing further. */
func (cursor *replayCursor) finish() {
	cursor.mounted = false
	cursor.fragments = cursor.fragments.Next()
	cursor.rehearsal.mutex.Lock()
	defer cursor.rehearsal.mutex.Unlock()
	cursor.rehearsal.progress.Passes++
}

/* unsupported counts an observation the worker could not be asked about. */
func (cursor *replayCursor) unsupported() {
	cursor.rehearsal.mutex.Lock()
	defer cursor.rehearsal.mutex.Unlock()
	cursor.rehearsal.progress.Unsupported++
}

/*
mark publishes one call onto the worker's visible track. A call is judged the
moment it is made, so it is published already answered: its position says when
the worker spoke and its value says how well that named the moment.
*/
func (cursor *replayCursor) mark(
	index int, decision *agent.Decision[Action], value float64, verdict string,
) {
	if decision.Action.Kind == "wait" || decision.Action.Kind == "hold" {
		return
	}
	cursor.mutex.Lock()
	defer cursor.mutex.Unlock()
	cursor.track.Marks = append(cursor.track.Marks, &wire.LearningMarkT{
		Id: decision.ID, Index: int32(index), Kind: decision.Action.Kind,
		Power: int32(decision.Action.Power), Reduce: decision.Action.Reduce,
		Value: value, Graded: true, Verdict: verdict,
	})
}
