package trader

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
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
syncInventory restores recovery state, enriches wallet lots, advances lifecycle,
and enables trading only after wallet readiness (and one-shot restore) complete.
*/
func (crypto *Crypto) syncInventory(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	crypto.healAbandonedExits(thesis)
	crypto.healCompletedEntries(thesis)

	snapshot := crypto.snapshot.Load()

	if snapshot != nil && crypto.balance != nil && crypto.balance.ModelReady() {
		if crypto.restoreRecovery(thesis, snapshot) {
			crypto.snapshot.CompareAndSwap(snapshot, nil)
		}
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
		crypto.desk.AdoptOpen()
	}

	// Trading stays gated until the one-shot recovery restore (which now also
	// reconciles against live open orders) clears the snapshot. A boot with no
	// recovery file starts with a nil snapshot and enables immediately.
	if crypto.balance != nil && crypto.balance.ModelReady() && crypto.snapshot.Load() == nil {
		crypto.trading.Store(true)
	}
}

/*
restoreRecovery applies durable holdings, reservations, and pending intents once
the live wallet is ready, then reconciles those intents against the exchange
open-order set. It reports whether reconcile was attempted so the caller only
clears the snapshot (and unblocks trading) after a successful pass.
*/
func (crypto *Crypto) restoreRecovery(
	thesis *types.Thesis,
	snapshot *types.Recovery,
) bool {
	if snapshot == nil || crypto.balance == nil {
		return false
	}

	for symbol, recovered := range snapshot.Holdings {
		crypto.balance.Enrich(symbol, recovered)
	}

	for _, row := range snapshot.Reservations {
		if row.ID == "" || row.Amount <= 0 {
			continue
		}

		_ = crypto.balance.RestoreClaim(
			row.ID, decimal.NewFromFloat64(row.Amount),
		)
	}

	for symbol, pending := range snapshot.PendingOrders {
		if symbol == "" {
			continue
		}

		phase := types.LifecycleEntrySubmitted

		if pending.Side == "sell" ||
			pending.Intent == broker.IntentExitPending ||
			pending.Intent == broker.IntentReducePending {
			phase = types.LifecycleExitSubmitted
		}

		thesis.NoteLifecycle(symbol, phase, thesis.At)
	}

	return crypto.reconcilePending()
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
		case types.LifecycleEvaluated, types.LifecycleExpired,
			types.LifecycleRejected, types.LifecycleInvalid:
			thesis.Lifecycle.Delete(symbol)
			return true
		case types.LifecycleClosed, types.LifecyclePostExitObservation,
			types.LifecyclePostMortemReady:
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
with no pending venue order. Scans live holdings only — not lifetime lifecycle.
*/
func (crypto *Crypto) healAbandonedExits(thesis *types.Thesis) {
	if crypto == nil || crypto.desk == nil || crypto.balance == nil ||
		thesis == nil || thesis.Lifecycle == nil {
		return
	}

	for holding := range crypto.balance.Holdings() {
		symbol := holding.Symbol
		value, found := thesis.Lifecycle.Load(symbol)

		if !found {
			continue
		}

		phase, _ := value.(string)

		if phase != types.LifecycleExitSubmitted {
			continue
		}

		if crypto.desk.Pending(symbol) {
			continue
		}

		if holding.Status == types.CLOSED {
			continue
		}

		if holding.Qty == nil || holding.Qty.Sign() <= 0 {
			continue
		}

		thesis.Lifecycle.Store(symbol, types.LifecycleManaging)
	}
}

/*
healCompletedEntries promotes EntrySubmitted lots that already show open wallet
qty with no pending entry order into managing.
*/
func (crypto *Crypto) healCompletedEntries(thesis *types.Thesis) {
	if crypto == nil || crypto.desk == nil || crypto.balance == nil ||
		thesis == nil || thesis.Lifecycle == nil {
		return
	}

	for holding := range crypto.balance.Holdings() {
		symbol := holding.Symbol
		value, found := thesis.Lifecycle.Load(symbol)

		if !found {
			continue
		}

		phase, _ := value.(string)

		if phase != types.LifecycleEntrySubmitted &&
			phase != types.LifecyclePartiallyEntered {
			continue
		}

		if crypto.desk.Pending(symbol) {
			continue
		}

		if holding.Status == types.CLOSED {
			continue
		}

		if holding.Qty == nil || holding.Qty.Sign() <= 0 {
			continue
		}

		thesis.NoteLifecycle(symbol, types.LifecycleEntered, thesis.At)
		thesis.Lifecycle.Store(symbol, types.LifecycleManaging)
	}
}

/*
markOpen places live wallet lots into the managing lifecycle on the durable
Thesis. In-flight entry/exit phases are left alone so stoploss and trade
submission do not re-fire every tick. Already-armed stops are never rebound.
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

			if stop := position.Stop(); stop != nil && stop.Armed() {
				holding.Stoploss = stop
			} else if holding.EntryPrice != nil {
				trail := 0.0

				if stop := position.Stop(); stop != nil {
					trail = stop.TrailDistance

					if trail <= 0 && stop.StopReturn < 0 {
						trail = -stop.StopReturn
					}
				}

				if trail <= 0 {
					trail = position.EntryTrail(holding)
				}

				if trail > 0 {
					position.BindStop(holding.EntryPrice.Float64(), trail)
					holding.Stoploss = position.Stop()
				}
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
