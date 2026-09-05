package strategy

import (
	"context"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
	"time"
)

/* LearningBook is the guarded resident spot book shared with live producers. */
type LearningBook interface {
	Book(string, func(*spotbook.Book))
}

/* Agent serializes market learning, capital decisions and operator inspection. */
type Agent struct {
	*LocalLearning
	*Execution
	*PolicyReview
	*LearningInspector
	Capital *CapitalLearner
	err     error
}

/* NewAgent wires the existing numerical, execution and journal dependencies. */
func NewAgent(ctx context.Context, grid *learning.Grid, books LearningBook,
	pair func(string) kraken.InstrumentPair, fee func(string) *kraken.TradeVolumeFee,
	initial *decimal.Decimal, record func(hindsight.LearningEvent) error,
) (*Agent, error) {
	if grid == nil || books == nil || pair == nil || fee == nil || initial == nil || initial.Sign() <= 0 || record == nil {
		return nil, errnie.Err(errnie.Validation, "learner: grid, book, venue rules, fees, positive capital and recorder are required", nil)
	}
	execution := &Execution{Skill: NewSkillMeter(AccountNone, time.Now()), Realization: NewRealizationMeter()}
	local := &LocalLearning{Knowledge: NewKnowledge(grid), Grid: grid, books: books, pair: pair, fee: fee,
		initial: initial.Copy(), Record: record, markets: make(map[string]*learningMarket), now: time.Now, execution: execution}
	capital := NewCapitalLearner(local)
	execution.Candidates = capital.Candidates
	review := &PolicyReview{ctx: ctx, local: local, reviews: make(chan []hindsight.Episode, 1), reviewed: make(map[string]struct{})}
	inspector := &LearningInspector{LocalLearning: local, Execution: execution, PolicyReview: review, Capital: capital, ctx: ctx, requests: make(chan learningRequest)}
	return &Agent{LocalLearning: local, Execution: execution, Capital: capital, PolicyReview: review, LearningInspector: inspector}, nil
}

/* Step runs the spot executable loop; futures can update the Grid but never wallets. */
func (agent *Agent) Step(envelope *types.Envelope) *types.Envelope {
	agent.steps++

	if agent.err == nil {
		agent.err = agent.Refresh(agent.now())
	}

	if candidate := agent.Capital.Candidates.current[agent.Grid.UpdatedLabel]; agent.err == nil && candidate != nil && candidate.Record.GridVersion != agent.Grid.Version {
		agent.err = agent.Capital.Candidates.Invalidate(agent.Grid.UpdatedLabel, agent.now(), "originating Grid row updated")
	}

	if envelope != nil && envelope.TypeID == types.EnvelopeLevel3 && agent.err == nil {
		agent.err = agent.advance(envelope.Level3Data, envelope.CaptureID)

		if agent.err == nil {
			agent.err = agent.Capital.Step(agent.LocalLearning, envelope.Level3Data.Symbol)
		}
	}

	if agent.err == nil {
		agent.err = agent.flush()
	}
	select {
	case request := <-agent.requests:
		request.reply <- agent.view(request.symbol)
	case episodes := <-agent.reviews:
		agent.review(episodes)
	default:
	}
	return envelope
}

/* Error reports a failed learning or recording operation to the workspace. */
func (agent *Agent) Error() error { return agent.err }
