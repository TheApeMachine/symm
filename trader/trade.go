package trader

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Trade submits Decide exits/reduces through Desk first, then enters, so freeing
slot intents are counted before BuyAfter. Session harnesses call this after Plan
to prove fill paths without requiring a full Market.Cut.
*/
func (crypto *Crypto) Trade(thesis *types.Thesis) {
	crypto.trade(thesis)
}

/*
trade submits Decide exits/reduces through Desk first, then enters, so freeing
slot intents are counted before BuyAfter.
*/
func (crypto *Crypto) trade(thesis *types.Thesis) {
	if crypto.desk == nil {
		return
	}

	enters := make(map[string]types.Decision, len(thesis.Decisions))
	sells := make(map[string]*decimal.Decimal, len(thesis.Decisions))
	epochs := map[string]uint64{}

	for _, decision := range thesis.Decisions {
		switch decision.Action {
		case types.ActionEnter:
			enters[decision.Symbol] = decision
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

	freeing := crypto.submitSells(thesis, sells, epochs)
	crypto.submitEnters(thesis, enters, epochs, freeing)
}

/*
submitSells places full exits and partial reduces on Desk-owned positions.
*/
func (crypto *Crypto) submitSells(
	thesis *types.Thesis,
	sells map[string]*decimal.Decimal,
	epochs map[string]uint64,
) int {
	freeing := 0

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

			// Lot gone or venue rejected size: stop re-arming ActionExit every tick.
			if _, open := crypto.desk.Position(symbol); !open {
				thesis.NoteLifecycle(symbol, types.LifecycleExitSubmitted, thesis.At)
			}

			continue
		}

		thesis.NoteLifecycle(symbol, types.LifecycleExitSubmitted, thesis.At)

		if quantity == nil {
			freeing++
		}
	}

	return freeing
}

/*
submitEnters places admitted Thesis holdings that still carry enter intent.
*/
func (crypto *Crypto) submitEnters(
	thesis *types.Thesis,
	enters map[string]types.Decision,
	epochs map[string]uint64,
	freeing int,
) {
	thesis.Holdings.Range(func(key, value any) bool {
		holding, ok := value.(*types.Holding)

		if !ok || holding == nil {
			return true
		}

		decision, enter := enters[holding.Symbol]

		if !enter {
			return true
		}

		if expired(thesis, holding.Symbol, epochs[holding.Symbol]) {
			crypto.release(decision)
			thesis.Holdings.Delete(key)
			return true
		}

		if err := errnie.Validate(holding); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"invalid holding for "+holding.Symbol,
				err,
			))

			crypto.release(decision)
			thesis.Holdings.Delete(key)
			return true
		}

		if !crypto.desk.HasSlotAfter(holding.IsOpportunity, freeing) {
			crypto.release(decision)
			return true
		}

		position, err := crypto.desk.BuyAfter(
			*holding, holding.IsOpportunity, freeing, decision.ReservationID,
		)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"failed to buy holding "+holding.Symbol,
				err,
			))

			crypto.release(decision)
			thesis.Holdings.Delete(key)

			return true
		}

		thesis.Positions.Store(holding.Symbol, position)
		thesis.NoteLifecycle(holding.Symbol, types.LifecycleEntrySubmitted, thesis.At)

		if freeing > 0 {
			freeing--
		}

		return true
	})
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
*/
func (crypto *Crypto) release(decision types.Decision) {
	if crypto.balance == nil {
		return
	}

	if decision.ReservationID != "" {
		_ = crypto.balance.Release(decision.ReservationID)
		return
	}

	if decision.ProposedNotional == nil || decision.ProposedNotional.Sign() <= 0 {
		return
	}

	_, _ = crypto.balance.Reserve(decision.ProposedNotional, nil, true)
}
