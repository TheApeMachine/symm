package strategy

import (
	"math/big"
	"time"

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
	Symbol    string
	At        time.Time
	MarketAt  time.Time
	Kind      types.Action
	Reduce    bool
	Quantity  *big.Rat
	Reference *big.Rat
	Mode      Mode
	Skill     SkillReading
}

/*
ExecutionDesk is the account a promoted policy lane reaches. It is deliberately
narrow: the agent decides what to do and how much, and the desk owns venue
mechanics. The agent never constructs venue orders itself, and a learning-only
run leaves this nil so no code path can produce one.
*/
type ExecutionDesk interface {
	Submit(ExecutionIntent) error
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

	if action.Kind == types.ActionHold || requested == nil || requested.Sign() <= 0 {
		return nil
	}

	reference := book.Asks.Low.Price.Rat()

	if action.Reduce {
		reference = book.Bids.High.Price.Rat()
	}

	intent := ExecutionIntent{
		Symbol: market.symbol, At: market.at, MarketAt: marketAt,
		Kind: action.Kind, Reduce: action.Reduce,
		Quantity:  new(big.Rat).Set(requested),
		Reference: new(big.Rat).Set(reference),
		Mode:      agent.Mode(),
		Skill:     agent.Skill.Reading(),
	}

	/*
		A refused or failed submission is an account-side fact, not a broken
		pipeline. Halting the workload on one would stop every symbol from
		learning because a single venue could not take a single order, and the
		lane whose measurement matters would go silent precisely when the
		account is behaving unexpectedly. The failure is counted and reported;
		the agent keeps observing.
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
