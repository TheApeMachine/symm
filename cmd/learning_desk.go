package cmd

import (
	"context"
	"math/big"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
executionQueue bounds the intents waiting on the venue. It absorbs a burst
while one order is in flight; it is not a backlog. An intent that waited behind
hundreds of others was priced against a book that no longer exists, so a full
queue drops rather than grows.
*/
const executionQueue = 64

/*
learningDesk routes the agent's policy intents to the configured account. It is
the only place an agent decision becomes an order, and it is attached only when
an account is configured, so a run without one cannot reach a venue.

Orders are placed on a dedicated goroutine, never on the caller's. The agent
runs inside the same workspace consumer that feeds the terminal, and a venue
round-trip taken there halts that consumer for its whole duration — the entire
dashboard stops the moment a position opens, and one hung request stops the
system for good. A single worker also keeps a symbol's intents in the order the
agent issued them.

The agent owns the whole trade life-cycle. It re-evaluates every open position
on each book update and issues its own EXIT, so nothing else holds protective
geometry: the desk carries the lot and executes what it is told.
*/
type learningDesk struct {
	desk       *broker.Desk
	instrument *broker.Instrument
	intents    chan strategy.ExecutionIntent

	submitted   atomic.Uint64
	unsupported atomic.Uint64
	diverged    atomic.Uint64
	dropped     atomic.Uint64
	failed      atomic.Uint64
	lastFailure atomic.Pointer[string]
}

/* newLearningDesk attaches the account; a nil desk attaches nothing. */
func newLearningDesk(
	ctx context.Context, desk *broker.Desk, instrument *broker.Instrument,
) strategy.ExecutionDesk {
	if desk == nil || instrument == nil {
		return nil
	}

	bridge := &learningDesk{
		desk: desk, instrument: instrument,
		intents: make(chan strategy.ExecutionIntent, executionQueue),
	}
	go bridge.run(ctx)

	return bridge
}

/*
Submit reconciles one policy intent against the account and queues it. It never
touches the venue: the caller is a workspace consumer, and every millisecond
spent here is a millisecond the whole pipeline is not moving.

The agent decides from its own simulated wallet, so its intent and the
account's actual position can disagree — an entry on a symbol the account
already holds, or an exit on one it never opened. That is reconciliation, not
failure: the intent is counted as diverged and the account is left alone.
*/
func (bridge *learningDesk) Submit(intent strategy.ExecutionIntent) error {
	if intent.Quantity == nil || intent.Quantity.Sign() <= 0 {
		return nil
	}

	// A reduction or exit needs inventory to give back; an entry needs the
	// account not to hold the symbol already, since the desk owns one lot per
	// symbol. Both are the same comparison from opposite sides.
	if intent.Reduce != (bridge.desk.Holding(intent.Symbol) > 0) {
		bridge.diverged.Add(1)
		return nil
	}

	if !intent.Reduce && intent.Kind != types.ActionEnter {
		bridge.unsupported.Add(1)
		return nil
	}

	select {
	case bridge.intents <- intent:
		return nil
	default:
		// The venue is slower than the agent is deciding. Dropping is the
		// honest outcome: this intent was priced against a book that will be
		// stale before the queue drains, and blocking here would stop every
		// symbol from learning while one order is in flight.
		bridge.dropped.Add(1)
		return nil
	}
}

/* Execution reports what the account did with the agent's intents. */
func (bridge *learningDesk) Execution() strategy.ExecutionStatus {
	status := strategy.ExecutionStatus{
		Submitted:   bridge.submitted.Load(),
		Unsupported: bridge.unsupported.Load(),
		Diverged:    bridge.diverged.Load(),
		Dropped:     bridge.dropped.Load(),
		Failed:      bridge.failed.Load(),
		Queued:      len(bridge.intents),
	}

	if failure := bridge.lastFailure.Load(); failure != nil {
		status.LastFailure = *failure
	}

	return status
}

/* run places queued intents one at a time, preserving the agent's order. */
func (bridge *learningDesk) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case intent := <-bridge.intents:
			if err := bridge.place(intent); err != nil {
				bridge.failed.Add(1)
				reason := err.Error()
				bridge.lastFailure.Store(&reason)
				errnie.Error(err)
			}
		}
	}
}

/*
place turns one reconciled intent into account action. Reductions give back
part of the lot, exits close it, and entries open one at the quantity the agent
fixed against the same displayed book it was measured on.
*/
func (bridge *learningDesk) place(intent strategy.ExecutionIntent) error {
	pair := bridge.instrument.Pair(intent.Symbol)

	if pair.Symbol == "" {
		bridge.unsupported.Add(1)
		return nil
	}

	// The account can move while an intent waits for the venue, so the
	// reconciliation that admitted it is re-checked against the state that
	// actually applies at placement time.
	if intent.Reduce != (bridge.desk.Holding(intent.Symbol) > 0) {
		bridge.diverged.Add(1)
		return nil
	}

	bridge.submitted.Add(1)

	if intent.Reduce {
		if intent.Kind == types.ActionExit {
			if err := bridge.desk.ManualExit(intent.Symbol); err != nil {
				return errnie.Err(
					errnie.IO, "symm: policy exit failed for "+intent.Symbol, err,
				)
			}

			return nil
		}

		volume := ratDecimal(intent.Quantity, pair.QtyPrecision)

		if volume == nil {
			bridge.unsupported.Add(1)
			return nil
		}

		if err := bridge.desk.Reduce(intent.Symbol, volume); err != nil {
			return errnie.Err(
				errnie.IO, "symm: policy reduction failed for "+intent.Symbol, err,
			)
		}

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

	if err := bridge.desk.Execute(*decision); err != nil {
		return errnie.Err(
			errnie.IO, "symm: policy entry failed for "+intent.Symbol, err,
		)
	}

	return nil
}

/*
ratDecimal converts an exact rational to the venue's own decimal precision.
The agent measures in exact rationals; the venue accepts decimals, and the
conversion happens once, here, at the boundary that actually requires it. An
unparseable value yields nil, which is refused rather than traded.
*/
func ratDecimal(value *big.Rat, precision int) *decimal.Decimal {
	converted, err := decimal.NewFromString(value.FloatString(precision))

	if err != nil {
		return nil
	}

	return converted
}
