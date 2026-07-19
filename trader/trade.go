package trader

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Trade submits Decide exits/reduces through Desk first, then enters. Challenger
entries wait for real slot capacity (fill-gated) — sell transport success alone
does not free a slot.
*/
func (crypto *Crypto) Trade(thesis *types.Thesis) {
	crypto.trade(thesis)
}

/*
trade submits exits first, then Arbiter-ordered enters against live slot count.
*/
func (crypto *Crypto) trade(thesis *types.Thesis) {
	if crypto.desk == nil || !crypto.trading.Load() {
		return
	}

	enters := make([]types.Decision, 0, len(thesis.Decisions))
	sells := make(map[string]*decimal.Decimal, len(thesis.Decisions))
	epochs := map[string]uint64{}

	for _, decision := range thesis.Decisions {
		switch decision.Action {
		case types.ActionEnter:
			enters = append(enters, decision)
			epochs[decision.Symbol] = decision.ValidThroughEpoch
		case types.ActionExit:
			sells[decision.Symbol] = nil
			epochs[decision.Symbol] = decision.ValidThroughEpoch
		case types.ActionReduce:
			if prior, ok := sells[decision.Symbol]; ok && prior == nil {
				continue
			}

			sells[decision.Symbol] = decision.ProposedQuantity
			epochs[decision.Symbol] = decision.ValidThroughEpoch
		}
	}

	crypto.submitSells(thesis, sells, epochs)
	crypto.submitEnters(thesis, enters, epochs)
}

/*
submitSells places full exits and partial reduces on Desk-owned positions.
*/
func (crypto *Crypto) submitSells(
	thesis *types.Thesis,
	sells map[string]*decimal.Decimal,
	epochs map[string]uint64,
) {
	for symbol, quantity := range sells {
		if expired(thesis, symbol, epochs[symbol]) {
			continue
		}

		if crypto.desk.Pending(symbol) {
			continue
		}

		if phase, found := thesis.Lifecycle.Load(symbol); found {
			if phase == types.LifecycleExitSubmitted {
				continue
			}
		}

		if err := crypto.desk.Sell(symbol, quantity); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"failed to sell "+symbol,
				err,
			))

			if _, open := crypto.desk.Position(symbol); !open {
				thesis.NoteLifecycle(symbol, types.LifecycleExitSubmitted, thesis.At)
			}

			continue
		}

		thesis.NoteLifecycle(symbol, types.LifecycleExitSubmitted, thesis.At)
	}
}

/*
submitEnters places admitted Thesis holdings in Arbiter admission order.
Capacity uses live OpenPositions only — no optimistic freeing from sell submit.
*/
func (crypto *Crypto) submitEnters(
	thesis *types.Thesis,
	enters []types.Decision,
	epochs map[string]uint64,
) {
	for _, decision := range enters {
		symbol := decision.Symbol

		raw, ok := thesis.Holdings.Load(symbol)

		if !ok {
			crypto.rejectEnter(thesis, decision, types.LifecycleRejected)
			continue
		}

		holding, ok := raw.(*types.Holding)

		if !ok || holding == nil {
			crypto.rejectEnter(thesis, decision, types.LifecycleRejected)
			continue
		}

		if expired(thesis, symbol, epochs[symbol]) {
			crypto.rejectEnter(thesis, decision, types.LifecycleExpired)
			continue
		}

		if err := errnie.Validate(holding); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"invalid holding for "+symbol,
				err,
			))

			crypto.rejectEnter(thesis, decision, types.LifecycleInvalid)
			continue
		}

		if !crypto.desk.HasSlot(holding.IsOpportunity) {
			// Rotation challengers keep claim+holding until the incumbent fill
			// frees a real slot. Fresh entries without a displace target demote.
			if decision.Displaces != "" {
				continue
			}

			crypto.rejectEnter(thesis, decision, types.LifecycleRejected)
			continue
		}

		position, err := crypto.desk.BuyAfter(
			*holding, holding.IsOpportunity, 0, decision.ReservationID,
		)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"failed to buy holding "+symbol,
				err,
			))

			crypto.rejectEnter(thesis, decision, types.LifecycleRejected)
			continue
		}

		thesis.Positions.Store(symbol, position)
		thesis.NoteLifecycle(symbol, types.LifecycleEntrySubmitted, thesis.At)
	}
}

/*
rejectEnter releases the claim, drops the Thesis candidate holding, and demotes
lifecycle so Opportunity.occupied cannot permanently block the symbol.
*/
func (crypto *Crypto) rejectEnter(
	thesis *types.Thesis,
	decision types.Decision,
	phase string,
) {
	crypto.release(decision)
	thesis.Holdings.Delete(decision.Symbol)
	thesis.NoteLifecycle(decision.Symbol, phase, thesis.At)
}

/*
expired rejects submission when the forecast epoch has advanced past validity.
*/
func expired(thesis *types.Thesis, symbol string, through uint64) bool {
	if through == 0 {
		return false
	}

	for _, forecast := range thesis.Forecasts {
		if forecast.Symbol == symbol && forecast.SourceEpoch > through {
			return true
		}
	}

	return false
}

/*
release returns Booked quote cash when entry submission fails or expires.
Claims are identity-keyed; amount-only rollback is not supported because the
exchange snapshot is never mutated by Book.
*/
func (crypto *Crypto) release(decision types.Decision) {
	if crypto.balance == nil || decision.ReservationID == "" {
		return
	}

	_ = crypto.balance.Release(decision.ReservationID)
}
