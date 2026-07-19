package trader

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Plan seeds open inventory, runs Planner.Decide, and publishes strategy frames.
*/
func (crypto *Crypto) Plan(thesis *types.Thesis) error {
	if thesis == nil || crypto.planner == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: plan requires thesis and planner",
			nil,
		))
	}

	crypto.seedOpen(thesis)
	crypto.planner.Decide(thesis)

	return nil
}

/*
seedOpen rebuilds Thesis.Holdings from the live wallet only. Durable recovery
may enrich entry economics on wallet-backed lots; it must never reintroduce a
symbol the exchange no longer holds.
*/
func (crypto *Crypto) seedOpen(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	crypto.healAbandonedExits()

	if crypto.snapshot != nil && crypto.balance != nil && crypto.balance.ModelReady() {
		for symbol, recovered := range crypto.snapshot.Holdings {
			crypto.balance.Enrich(symbol, recovered)
		}

		// One-shot: never reintroduce recovered lots after wallet has spoken.
		crypto.snapshot = nil
	}

	thesis.Holdings.Range(func(key, value any) bool {
		thesis.Holdings.Delete(key)
		return true
	})

	live := map[string]struct{}{}

	if crypto.balance != nil {
		for holding := range crypto.balance.Holdings() {
			seed := holding
			live[holding.Symbol] = struct{}{}
			thesis.Holdings.Store(holding.Symbol, &seed)
			crypto.markOpen(thesis, &seed)
		}
	}

	for symbol, phase := range crypto.phases {
		if _, ok := live[symbol]; ok {
			continue
		}

		if phase == types.LifecycleClosed {
			continue
		}

		thesis.NoteLifecycle(symbol, types.LifecycleClosed, thesis.At)
		crypto.phases[symbol] = types.LifecycleClosed
	}

	if crypto.desk != nil {
		_ = crypto.desk.OpenPositions()
	}
}

/*
healAbandonedExits re-arms ExitSubmitted symbols that still hold open inventory
with no pending venue order. Yesterday's failed paper exits left the lifecycle
rail stuck on EXIT SUBMITTED forever because submitSells skips that phase and
Regulate will not re-append ActionExit while it remains set.
*/
func (crypto *Crypto) healAbandonedExits() {
	if crypto == nil || crypto.desk == nil || crypto.balance == nil ||
		len(crypto.phases) == 0 {
		return
	}

	for symbol, phase := range crypto.phases {
		if phase != types.LifecycleExitSubmitted {
			continue
		}

		if crypto.desk.Pending(symbol) {
			continue
		}

		holding, err := crypto.balance.Holding(symbol)

		if err != nil || holding.Status == types.CLOSED {
			continue
		}

		if holding.Qty == nil || holding.Qty.Sign() <= 0 {
			continue
		}

		crypto.phases[symbol] = types.LifecycleManaging
	}
}

/*
markOpen places wallet-backed inventory into the managing lifecycle and journals
a one-shot entered observation so the audit rail can explain restart inventory.
In-flight entry/exit phases are restored onto each fresh Thesis so stoploss and
trade submission do not re-fire every tick.
*/
func (crypto *Crypto) markOpen(thesis *types.Thesis, holding *types.Holding) {
	if thesis == nil || holding == nil || holding.Symbol == "" {
		return
	}

	if crypto.phases == nil {
		crypto.phases = map[string]string{}
	}

	prior := crypto.phases[holding.Symbol]

	if prior == "" {
		at := thesis.At

		if holding.EntryAt != nil {
			at = holding.EntryAt.UTC()
		}

		thesis.NoteLifecycle(holding.Symbol, types.LifecycleEntered, at)
		crypto.phases[holding.Symbol] = types.LifecycleEntered
		prior = types.LifecycleEntered
	}

	if holding.Stoploss != nil && holding.EntryPrice != nil {
		trail := holding.Stoploss.TrailDistance

		if trail <= 0 && holding.Stoploss.StopReturn < 0 {
			trail = -holding.Stoploss.StopReturn
		}

		holding.Stoploss.Bind(holding.EntryPrice.Float64(), trail)
	}

	switch prior {
	case types.LifecycleExitSelected, types.LifecycleExitSubmitted,
		types.LifecycleEntrySubmitted, types.LifecyclePartiallyExited,
		types.LifecyclePartiallyEntered:
		thesis.Lifecycle.Store(holding.Symbol, prior)

		return
	}

	thesis.Lifecycle.Store(holding.Symbol, types.LifecycleManaging)
	crypto.phases[holding.Symbol] = types.LifecycleManaging
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
