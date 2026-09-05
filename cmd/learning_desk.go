package cmd

import (
	"math/big"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
learningDesk routes the agent's policy intents to the configured account. It
is the only place an agent decision becomes an order, and it is attached only
when an account is configured, so a run without one cannot reach a venue.

The agent owns the whole trade life-cycle. It re-evaluates every open position
on each book update and issues its own EXIT, so nothing else holds protective
geometry: the desk carries the lot and executes what it is told.
*/
type learningDesk struct {
	desk        *broker.Desk
	instrument  *broker.Instrument
	submitted   atomic.Uint64
	unsupported atomic.Uint64
	diverged    atomic.Uint64
	failed      atomic.Uint64
}

/* newLearningDesk attaches the account; a nil desk attaches nothing. */
func newLearningDesk(desk *broker.Desk, instrument *broker.Instrument) strategy.ExecutionDesk {
	if desk == nil || instrument == nil {
		return nil
	}

	return &learningDesk{desk: desk, instrument: instrument}
}

/*
Submit turns one policy intent into account action. Reductions close the open
position; entries open one at the quantity the agent fixed against the same
displayed book it was measured on.

The agent decides from its own simulated wallet, so its intent and the
account's actual position can disagree — an entry the account already holds, or
an exit on a symbol it never opened. That is reconciliation, not failure: the
intent is counted as diverged and the account is left alone.
*/
func (bridge *learningDesk) Submit(intent strategy.ExecutionIntent) error {
	if intent.Quantity == nil || intent.Quantity.Sign() <= 0 {
		return nil
	}

	if intent.Reduce {
		if intent.Kind != types.ActionExit {
			bridge.unsupported.Add(1)
			return nil
		}

		// The agent decides from its own simulated wallet, which is not the
		// account. When the account holds nothing there is nothing to close,
		// and that is a reconciliation fact, not a failure: the intent is
		// counted as diverged and the account is left as it is.
		if bridge.desk.Holding(intent.Symbol) <= 0 {
			bridge.diverged.Add(1)
			return nil
		}

		bridge.submitted.Add(1)

		if err := bridge.desk.ManualExit(intent.Symbol); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "symm: policy exit failed for "+intent.Symbol, err,
			))
		}

		return nil
	}

	if intent.Kind != types.ActionEnter {
		bridge.unsupported.Add(1)
		return nil
	}

	// The desk already owns one lot per symbol. An entry the account cannot
	// take is divergence between the simulated wallet and the account, not an
	// execution failure.
	if bridge.desk.Holding(intent.Symbol) > 0 {
		bridge.diverged.Add(1)
		return nil
	}

	pair := bridge.instrument.Pair(intent.Symbol)

	if pair.Symbol == "" {
		bridge.unsupported.Add(1)
		return nil
	}

	quantity := ratDecimal(intent.Quantity, pair.QtyPrecision)
	reference := ratDecimal(intent.Reference, pair.PricePrecision)

	if quantity == nil || reference == nil {
		bridge.unsupported.Add(1)
		return nil
	}

	decision := types.NewDecision(types.ActionEnter, intent.Symbol)
	decision.At = intent.At
	decision.AllocationClass = "capital"
	decision.Cause = "policy_edge"
	decision.Reason = intent.Skill.Reason
	decision.Confidence = intent.Skill.Confidence
	decision.TaskSkill, decision.TaskSkillReady = intent.Skill.Mean, intent.Skill.Qualified
	decision.ProposedQuantity = quantity
	decision.ReferencePrice = reference
	decision.ProposedNotional = decimal.NewFromInt64(0).Add(quantity).Mul(reference)
	decision.AvailableCapital = bridge.desk.Cash()
	decision.OpenPositions = bridge.desk.OpenPositions()
	bridge.submitted.Add(1)

	if err := bridge.desk.Execute(*decision); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "symm: policy entry failed for "+intent.Symbol, err,
		))
	}

	return nil
}

/*
ratDecimal converts an exact rational to the venue's own decimal precision.
The agent measures in exact rationals; the venue accepts decimals, and the
conversion happens once, here, at the boundary that actually requires it. An
unparseable value yields nil, which the desk rejects rather than trades.
*/
func ratDecimal(value *big.Rat, precision int) *decimal.Decimal {
	converted, err := decimal.NewFromString(value.FloatString(precision))

	if err != nil {
		return nil
	}

	return converted
}
