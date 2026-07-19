package trader

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Plan syncs wallet lifecycle onto the durable Thesis, runs Planner.Decide, and
publishes strategy frames. Thesis.Holdings stays Admit-created only.
*/
func (crypto *Crypto) Plan(thesis *types.Thesis) error {
	if thesis == nil || crypto.planner == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: plan requires thesis and planner",
			nil,
		))
	}

	crypto.syncInventory(thesis)
	crypto.planner.Decide(thesis)

	return nil
}

/*
syncInventory enriches Balance from recovery, restores lifecycle for live wallet
lots, and advances flat symbols through close and PostMortem. It never copies
wallet inventory onto Thesis.Holdings.
*/
func (crypto *Crypto) syncInventory(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	crypto.healAbandonedExits(thesis)

	if crypto.snapshot != nil && crypto.balance != nil && crypto.balance.ModelReady() {
		for symbol, recovered := range crypto.snapshot.Holdings {
			crypto.balance.Enrich(symbol, recovered)
		}

		crypto.snapshot = nil
	}

	live := map[string]struct{}{}

	if crypto.balance != nil {
		for holding := range crypto.balance.Holdings() {
			lot := holding
			live[holding.Symbol] = struct{}{}
			crypto.markOpen(thesis, &lot)
		}
	}

	crypto.closeMissing(thesis, live)

	if crypto.desk != nil {
		_ = crypto.desk.OpenPositions()
	}
}

/*
closeMissing advances lifecycle for symbols no longer in the wallet and runs
PostMortem when ready.
*/
func (crypto *Crypto) closeMissing(
	thesis *types.Thesis,
	live map[string]struct{},
) {
	if thesis == nil || thesis.Lifecycle == nil {
		return
	}

	thesis.Lifecycle.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		if _, open := live[symbol]; open {
			return true
		}

		phase, _ := value.(string)

		switch phase {
		case types.LifecycleClosed, types.LifecyclePostExitObservation,
			types.LifecyclePostMortemReady, types.LifecycleEvaluated,
			types.LifecycleExpired, types.LifecycleRejected, types.LifecycleInvalid:
			return true
		}

		thesis.NoteLifecycle(symbol, types.LifecycleClosed, thesis.At)
		thesis.NoteLifecycle(symbol, types.LifecyclePostMortemReady, thesis.At)

		if crypto.postMortem != nil {
			errnie.Error(crypto.postMortem.Evaluate(thesis, symbol))
		}

		return true
	})
}

/*
healAbandonedExits re-arms ExitSubmitted symbols that still hold open inventory
with no pending venue order.
*/
func (crypto *Crypto) healAbandonedExits(thesis *types.Thesis) {
	if crypto == nil || crypto.desk == nil || crypto.balance == nil ||
		thesis == nil || thesis.Lifecycle == nil {
		return
	}

	thesis.Lifecycle.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		phase, _ := value.(string)

		if phase != types.LifecycleExitSubmitted {
			return true
		}

		if crypto.desk.Pending(symbol) {
			return true
		}

		holding, err := crypto.balance.Holding(symbol)

		if err != nil || holding.Status == types.CLOSED {
			return true
		}

		if holding.Qty == nil || holding.Qty.Sign() <= 0 {
			return true
		}

		thesis.Lifecycle.Store(symbol, types.LifecycleManaging)

		return true
	})
}

/*
markOpen places live wallet lots into the managing lifecycle on the durable
Thesis. In-flight entry/exit phases are left alone so stoploss and trade
submission do not re-fire every tick. Stoploss Bind runs on Position.
*/
func (crypto *Crypto) markOpen(thesis *types.Thesis, holding *types.Holding) {
	if thesis == nil || holding == nil || holding.Symbol == "" {
		return
	}

	priorValue, found := thesis.Lifecycle.Load(holding.Symbol)
	prior, _ := priorValue.(string)

	if !found || prior == "" {
		at := thesis.At

		if holding.EntryAt != nil {
			at = holding.EntryAt.UTC()
		}

		thesis.NoteLifecycle(holding.Symbol, types.LifecycleEntered, at)
		prior = types.LifecycleEntered
	}

	if crypto.desk != nil {
		if position, ok := crypto.desk.Position(holding.Symbol); ok {
			position.TakeStop(holding)

			if holding.EntryPrice != nil {
				trail := 0.0

				if stop := position.Stop(); stop != nil {
					trail = stop.TrailDistance

					if trail <= 0 && stop.StopReturn < 0 {
						trail = -stop.StopReturn
					}
				}

				position.BindStop(holding.EntryPrice.Float64(), trail)
				holding.Stoploss = position.Stop()
			}
		}
	}

	switch prior {
	case types.LifecycleExitSelected, types.LifecycleExitSubmitted,
		types.LifecycleEntrySubmitted, types.LifecyclePartiallyExited,
		types.LifecyclePartiallyEntered:
		thesis.Lifecycle.Store(holding.Symbol, prior)

		return
	}

	thesis.Lifecycle.Store(holding.Symbol, types.LifecycleManaging)
}

/*
constraints returns quote capital plus normal and reserved slot ceilings.
*/
func (crypto *Crypto) constraints() (float64, int, int, error) {
	normal, reserved := 0, 0

	if crypto.desk != nil {
		normal = crypto.desk.NormalSlots()
		reserved = crypto.desk.ReservedSlots()
	}

	if crypto.balance == nil {
		return 0, normal, reserved, errnie.Error(errnie.Err(
			errnie.NotFound,
			"crypto: balance unavailable for plan",
			nil,
		))
	}

	available, err := crypto.balance.AvailableQuote()

	if err != nil {
		return 0, normal, reserved, errnie.Error(errnie.Err(
			errnie.NotFound,
			"crypto: quote balance unavailable for plan",
			err,
		))
	}

	return available, normal, reserved, nil
}

/*
publishStrategy forwards decision and lifecycle frames so the terminal sees
strategy output after Analyzer already published forecasts and graphs.
*/
func (crypto *Crypto) publishStrategy(thesis *types.Thesis) {
	if crypto.uiHub == nil || thesis == nil {
		return
	}

	if len(thesis.Decisions) > 0 {
		crypto.emit(datura.Map[any]{"decisions": thesis.Decisions})
	}

	lifecycle := make([]datura.Map[any], 0)

	thesis.Lifecycle.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		status, ok := value.(string)

		if !ok || status == "" {
			return true
		}

		lifecycle = append(lifecycle, datura.Map[any]{
			"symbol": symbol,
			"state":  status,
		})

		return true
	})

	if len(lifecycle) > 0 {
		crypto.emit(datura.Map[any]{"lifecycle": lifecycle})
	}

	if len(thesis.Findings) > 0 {
		crypto.emit(datura.Map[any]{"findings": thesis.Findings})
	}

	if len(thesis.Categories) > 0 {
		crypto.emit(datura.Map[any]{"categories": thesis.Categories})
	}
}

/*
emit enqueues one UI frame without blocking the trade path. Empty marshal
results (non-finite payloads) are dropped so the browser never sees truncated JSON.
*/
func (crypto *Crypto) emit(frame datura.Map[any]) {
	if crypto.uiHub == nil || frame == nil {
		return
	}

	payload := frame.Marshal()

	if len(payload) == 0 {
		return
	}

	select {
	case crypto.uiHub.Messages <- payload:
	default:
	}
}
