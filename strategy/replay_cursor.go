package strategy

import (
	"container/ring"
	"context"
	"maps"
	"sort"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
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
	rehearsal    *Rehearsal
	ctx          context.Context
	mutex        sync.Mutex
	pending      [][]hindsight.Observation
	fragments    *ring.Ring
	learned      *cognition.Engine
	columns      [][2]string
	observations []hindsight.Observation
	space        *grid.Space
	source       *recordedBook
	trader       *Trader
	member       *agent.Agent[Action]
	evaluations  []*Evaluation
	track        wire.LearningTrackT
	index        int
	idle         time.Duration
	lastProfit   *decimal.Decimal
	lastAt       time.Time
	settled      bool
}

func (cursor *replayCursor) Step(envelope *types.Envelope) *types.Envelope {
	if cursor.ctx.Err() != nil || cursor.rehearsal.Error() != nil {
		return envelope
	}

	if cursor.observations == nil {
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

	if cursor.index < len(cursor.observations) {
		err = cursor.advance()
	}

	if err == nil && cursor.index == len(cursor.observations) {
		err = cursor.finish()
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
	for _, fragment := range cursor.pending {
		addition.Value = fragment
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
	cursor.observations = cursor.fragments.Value.([]hindsight.Observation)
	cursor.space = grid.NewSpace()
	for _, identity := range cursor.columns {
		cursor.space.Column(identity[0], identity[1])
	}
	cursor.source = &recordedBook{}
	price := broker.NewRecordedPrice(cursor.rehearsal.price, cursor.source)
	trader, err := NewTrader(cursor.rehearsal.learner.Traders[0].api, price,
		cursor.rehearsal.learner.Traders[0].Balance.Quote, cursor.rehearsal.learner.funding)

	if err != nil {
		return errnie.Error(err)
	}
	cursor.trader = trader
	cursor.member, err = agent.New(cursor.ctx, trader, cursor.learned, true)
	cursor.index, cursor.idle, cursor.settled = 0, 0, false
	cursor.lastProfit, cursor.lastAt = nil, time.Time{}
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
	observations := cursor.observations
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

func (cursor *replayCursor) advance() error {
	index := cursor.index
	cursor.index++
	cursor.mutex.Lock()
	cursor.track.Index = int32(cursor.index)
	cursor.mutex.Unlock()
	observation := cursor.observations[index]
	cursor.source.Step(observation)
	cursor.trader.At, cursor.trader.Version = observation.ReceivedAt, uint64(index+1)
	input := observation
	measurements := make([]*data.Measurement[float64], 0, len(input.Measurements))
	from := observation.ReceivedAt
	for _, captured := range input.Measurements {
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

	if len(measurements) > 0 {
		if err := cursor.space.Step(measurements); err != nil {
			return errnie.Error(err)
		}
	}

	if !cursor.trader.Ready(observation.Symbol) {
		cursor.member.Transition(runtime.WAITING)
		cursor.lastProfit = nil // Missing valuation cannot establish an idle interval.
		return nil
	}
	mark, err := cursor.trader.Objective()

	if err != nil || mark == nil {
		cursor.member.Transition(runtime.WAITING)
		cursor.lastProfit = nil
		return errnie.Error(err)
	}

	if cursor.lastProfit != nil && cursor.trader.Profit.Cmp(cursor.lastProfit) <= 0 {
		cursor.idle += observation.ReceivedAt.Sub(cursor.lastAt)
	}
	cursor.lastProfit, cursor.lastAt = cursor.trader.Profit, observation.ReceivedAt

	// Quotes advance the wallet clock even when no precursor was captured.
	// They cannot activate an absent row or reuse a previous grid impulse.
	if len(measurements) == 0 {
		cursor.member.Transition(runtime.WAITING)
		cursor.rehearsal.mutex.Lock()
		cursor.rehearsal.progress.Unsupported++
		cursor.rehearsal.mutex.Unlock()
		return nil
	}
	impulse, err := cursor.space.Impulse(observation.Symbol, observation.ReceivedAt, from)

	if err != nil {
		return errnie.Error(err)
	}

	if !impulse.Ready || index == len(cursor.observations)-1 {
		cursor.member.Transition(runtime.WAITING)
		cursor.rehearsal.mutex.Lock()
		cursor.rehearsal.progress.Unsupported++
		cursor.rehearsal.mutex.Unlock()
		return nil
	}
	before, entry := cursor.member.Decisions, -1
	baseline, realized := cursor.trader.Profit, cursor.trader.Realized

	if opened, holding := cursor.trader.Opened[observation.Symbol]; holding {
		entry = sort.Search(len(cursor.observations), func(index int) bool {
			return !cursor.observations[index].ReceivedAt.Before(opened)
		})
	}

	cursor.member.Step(impulse)

	if err := cursor.member.Error(); err != nil {
		return errnie.Error(err)
	}

	if cursor.member.Decisions == before {
		return nil
	}
	decision := cursor.member.Last
	evaluation := cursor.trader.Evaluations[observation.Symbol][decision.ID]
	evaluation.Index, evaluation.Entry = index, entry
	evaluation.Baseline, evaluation.Idle = baseline, cursor.idle

	if decision.Action.Reduce {
		evaluation.Secured = cursor.trader.Positions[observation.Symbol].Holding.RealizedPnL.Sub(realized)
	}
	cursor.evaluations = append(cursor.evaluations, evaluation)

	// Waiting is the absence of an action and is already the whole rest of the
	// track. Only decisions that reach the account are positioned on it.
	if decision.Action.Kind != "wait" {
		cursor.mutex.Lock()
		cursor.track.Marks = append(cursor.track.Marks, &wire.LearningMarkT{
			Id: decision.ID, Index: int32(index), Kind: decision.Action.Kind,
			Power: int32(decision.Action.Power), Reduce: decision.Action.Reduce,
		})
		cursor.mutex.Unlock()
	}
	return nil
}

func (cursor *replayCursor) finish() error {
	if !cursor.settled {
		last := cursor.observations[len(cursor.observations)-1]
		if err := cursor.trader.End(last.Symbol, last.ReceivedAt); err != nil {
			return errnie.Error(err)
		}
		cursor.settled = true
	}

	if len(cursor.evaluations) == 0 {
		cursor.columns = cursor.space.Columns
		cursor.observations = nil
		cursor.fragments = cursor.fragments.Next()
		cursor.rehearsal.mutex.Lock()
		cursor.rehearsal.progress.Passes++
		cursor.rehearsal.mutex.Unlock()
		return nil
	}
	evaluation := cursor.evaluations[0]

	if evaluation.Secured == nil && cursor.trader.open() == 0 {
		evaluation.Secured = cursor.trader.Balance.Cash().Sub(cursor.trader.Initial).Sub(evaluation.Baseline)
	}

	if evaluation.Secured == nil {
		// Unreleased basis is explicitly a capital-completion debit. It is not
		// a fabricated zero mark or a realized loss on remaining inventory.
		holding := cursor.trader.Positions[evaluation.Label].Holding
		evaluation.Secured = decimal.NewFromInt64(0).Sub(holding.Basis).Sub(holding.EntryFee)
	}
	evaluation.Idle = cursor.idle - evaluation.Idle

	if err := evaluation.Grade(cursor.observations, cursor.rehearsal.price); err != nil {
		return errnie.Error(err)
	}

	if _, err := cursor.member.Resolve(evaluation.ID, evaluation.Value); err != nil {
		return errnie.Error(err)
	}

	if err := cursor.rehearsal.learner.Learn(evaluation.Label, cursor.space.Columns, evaluation.Context,
		evaluation.Action, evaluation.Value, evaluation.Authority); err != nil {
		return errnie.Error(err)
	}
	cursor.grade(evaluation)
	cursor.evaluations = cursor.evaluations[1:]
	cursor.rehearsal.mutex.Lock()
	defer cursor.rehearsal.mutex.Unlock()
	cursor.rehearsal.progress.Decisions++
	cursor.rehearsal.progress.Trained++
	cursor.rehearsal.progress.LastSymbol = evaluation.Label
	cursor.rehearsal.progress.LastAction = evaluation.Action.Kind
	cursor.rehearsal.progress.LastReturn = evaluation.Value
	cursor.rehearsal.progress.LastFailure = evaluation.Failure
	return nil
}

// grade returns a published mark's measured outcome to the track it was taken
// on, so an arrow that has been answered reads differently from one that has
// not. A mark whose fragment has already been replaced is simply absent.
func (cursor *replayCursor) grade(evaluation *Evaluation) {
	cursor.mutex.Lock()
	defer cursor.mutex.Unlock()

	for _, mark := range cursor.track.Marks {
		if mark.Id == evaluation.ID {
			mark.Value, mark.Graded = evaluation.Value, true
			return
		}
	}
}
