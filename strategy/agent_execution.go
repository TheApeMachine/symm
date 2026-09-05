package strategy

import (
	"math/big"
	"time"

	"github.com/google/uuid"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
ExecutionIntent is one policy decision expressed in account terms, ready for a
venue. Quantity is the amount the policy lane fixed against the same displayed
book it was measured on, in the instrument's own precision.

Reduce distinguishes a decision that sells inventory from one that acquires
it. Mode records the authority under which the intent was produced, so an
order can never be attributed to a lower authority than the one that made it.
*/
type ExecutionIntent struct {
	CorrelationID string
	Symbol        string
	At            time.Time
	MarketAt      time.Time
	Kind          types.Action
	Reduce        bool
	Quantity      *big.Rat
	Reference     *big.Rat
	Mode          Mode
	Skill         SkillReading
}

/*
ExecutionDesk is the account the policy lane reaches. It is deliberately
narrow: the agent decides what to do and how much, and the desk owns venue
mechanics. The agent never constructs venue orders itself, and a learning-only
run leaves this nil so no code path can produce one.

Submit must not talk to the venue. It is called from the workspace consumer
that also feeds the terminal, so a round-trip taken inside it stops the whole
pipeline for its duration. Implementations queue the intent and place it
elsewhere.
*/
type ExecutionDesk interface {
	Submit(ExecutionIntent) error
}

/*
RealizationFeedback connects an execution desk's asynchronous placement and fill
feedback to the agent's realization circuit breaker.
*/
type RealizationFeedback interface {
	AttachRealization(*RealizationMeter)
}

/*
ExecutionStatus is what the account did with the agent's intents. Because
placement happens off the deciding path, an outcome is a report rather than a
return value, and every category here is a distinct fact: Diverged means the
account disagreed with the simulated wallet, Dropped means the venue was slower
than the agent was deciding, and Failed means an order was actually refused.
*/
type ExecutionStatus struct {
	Submitted   uint64 `json:"submitted"`
	Unsupported uint64 `json:"unsupported"`
	Diverged    uint64 `json:"diverged"`
	Dropped     uint64 `json:"dropped"`
	Failed      uint64 `json:"failed"`
	Queued      int    `json:"queued"`
	LastFailure string `json:"lastFailure,omitempty"`
}

/*
ExecutionReporter is an account that can describe its own outcomes. It is
optional: an agent whose desk cannot report simply shows nothing rather than
inventing a status.
*/
type ExecutionReporter interface {
	Execution() ExecutionStatus
}

/*
dispatch forwards a policy decision to the account when measured skill has
earned the authority to act. Waiting is never dispatched: an account that is
not asked to do anything is the same account either way, and sending "hold"
to a venue would be an order that does not exist.

Every guard here is a refusal to act, never a promotion: no measurement can
grant an authority the configuration did not already allow.
*/
func (agent *Agent) dispatch(
	market *learningMarket,
	action LearningAction,
	requested *big.Rat,
	book *spotbook.Book,
	marketAt time.Time,
) error {
	if agent.Desk == nil || agent.Mode() == ModeLearning {
		return nil
	}

	if agent.Realization != nil && !agent.Realization.AllowsTrading() {
		return nil
	}

	if action.Kind == types.ActionHold || requested == nil || requested.Sign() <= 0 {
		return nil
	}

	reference := book.Asks.Low.Price.Rat()

	if action.Reduce {
		reference = book.Bids.High.Price.Rat()
	}

	correlationID := uuid.NewString()

	intent := ExecutionIntent{
		CorrelationID: correlationID,
		Symbol:        market.symbol,
		At:            market.at,
		MarketAt:      marketAt,
		Kind:          action.Kind,
		Reduce:        action.Reduce,
		Quantity:      new(big.Rat).Set(requested),
		Reference:     new(big.Rat).Set(reference),
		Mode:          agent.Mode(),
		Skill:         agent.Skill.Reading(),
	}

	/*
		A refused or failed submission is an account-side fact, not a broken
		pipeline. Halting the workload on one would stop every symbol from
		learning because a single venue could not take a single order, and the
		lane whose measurement matters would go silent precisely when the
		account is behaving unexpectedly. The failure is counted and reported;
		the agent keeps observing.

		Realization does not observe submission here because Submit is only the
		synchronous queue handoff; actual venue placement and fill outcomes are
		observed asynchronously by the execution desk.
	*/
	if err := agent.Desk.Submit(intent); err != nil {
		agent.rejected++

		agent.lastRejection = errnie.Error(errnie.Err(
			errnie.IO,
			"[agent] policy intent was not accepted by the account",
			err,
		))

		return nil
	}

	agent.dispatched++
	return nil
}
