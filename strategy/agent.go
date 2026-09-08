package strategy

import (
	"context"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/*
LearningBook is the guarded resident spot book shared with live producers.
*/
type LearningBook interface {
	Book(string, func(*spotbook.Book))
}

/*
Agent serializes market learning, capital decisions and operator inspection.
*/
type Agent struct {
	*LocalLearning
	*Execution
	*PolicyReview
	*LearningInspector
	Capital *CapitalLearner
	err     error
}

/*
NewAgent wires the existing numerical, execution and journal dependencies.
*/
func NewAgent(
	ctx context.Context,
	grid *learning.Grid,
	books LearningBook,
	price *broker.Price,
	initial *decimal.Decimal,
	record func(hindsight.LearningEvent) error,
) (*Agent, error) {
	if err := errnie.Require(map[string]any{
		"ctx":     ctx,
		"grid":    grid,
		"books":   books,
		"price":   price,
		"initial": initial,
		"record":  record,
	}); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[agent] not all requirements met",
			err,
		))
	}

	execution := &Execution{
		Skill: NewSkillMeter(
			AccountNone, time.Now(),
		),
		Realization: NewRealizationMeter(),
	}

	local := &LocalLearning{
		Knowledge: NewKnowledge(grid),
		Grid:      grid,
		books:     books,
		price:     price,
		initial:   initial.Copy(),
		Record:    record,
		markets:   make(map[string]*learningMarket),
		now:       time.Now,
		execution: execution,
	}

	local.Desk = NewDesk(local.Knowledge, price, initial)

	capital := NewCapitalLearner(local)
	execution.Candidates = capital.Candidates

	review := &PolicyReview{
		ctx:      ctx,
		local:    local,
		reviews:  make(chan []hindsight.Episode, 1),
		reviewed: make(map[string]struct{}),
	}

	inspector := &LearningInspector{
		LocalLearning: local,
		Execution:     execution,
		PolicyReview:  review,
		Capital:       capital,
		ctx:           ctx,
		requests:      make(chan learningRequest),
	}

	return &Agent{
		LocalLearning:     local,
		Execution:         execution,
		Capital:           capital,
		PolicyReview:      review,
		LearningInspector: inspector,
	}, nil
}

/* Step runs the spot executable loop; futures can update the Grid but never wallets. */
func (agent *Agent) Step(envelope *types.Envelope) *types.Envelope {
	agent.steps++

	if agent.err == nil {
		agent.err = agent.Refresh(agent.now())
	}

	if candidate := agent.Capital.Candidates.current[agent.Grid.UpdatedLabel]; agent.err == nil &&
		candidate != nil &&
		candidate.Record.GridVersion != agent.Grid.Version {
		agent.err = agent.Capital.Candidates.Invalidate(
			agent.Grid.UpdatedLabel, agent.now(), "originating Grid row updated",
		)
	}

	if envelope != nil && envelope.TypeID == types.EnvelopeLevel3 && agent.err == nil {
		agent.err = agent.advance(envelope.Level3Data, envelope.CaptureID)

		if agent.err == nil {
			agent.err = agent.Capital.Step(
				agent.LocalLearning, envelope.Level3Data.Symbol,
			)
		}
	}

	if agent.err == nil {
		agent.err = agent.flush()
	}

	select {
	case request := <-agent.requests:
		if request.checkpoint != nil {
			request.checkpoint <- agent.Knowledge.Model.Checkpoint()
			break
		}

		request.reply <- agent.view(request.symbol)
	case episodes := <-agent.reviews:
		agent.review(episodes)
	default:
	}

	return envelope
}

/*
Error reports a failed learning or recording operation to the workspace.
*/
func (agent *Agent) Error() error { return errnie.Error(agent.err) }

/*
RoundTripFees is what the venue charges to open and close a position in one
symbol, on both legs, as a fraction of notional.

It is the part of a round trip that is knowable without a book. A historical
episode has no retained spread, so this is what its evidence can honestly be
stated net of — an understatement of the true cost by whatever the spread was,
which is better than stating it net of nothing.
*/
func (agent *Agent) RoundTripFees(symbol string) float64 {
	fee := agent.LocalLearning.price.FeeIfAvailable(symbol)

	if fee == nil || fee.Fee == nil {
		return 0
	}

	return 2 * fee.Fee.Float64() / 100
}

/*
Checkpoint captures the shared memory the desk has built, for persistence.
*/
func (agent *Agent) Checkpoint() Checkpoint {
	return agent.Knowledge.Model.Checkpoint()
}

/*
Restore loads a previously saved memory into the desk, so every trader begins
from what the desk collectively learned rather than from nothing.

The system still returns to learning after a restart. What changes is where it
starts from: the traders open with the memory the tape has already taught, and
develop it further rather than rediscovering it.
*/
func (agent *Agent) Restore(checkpoint Checkpoint) error {
	return agent.Knowledge.Model.Restore(checkpoint)
}

/* Desk exposes the traders for inspection. */
func (agent *Agent) Desk() *Desk { return agent.LocalLearning.Desk }
