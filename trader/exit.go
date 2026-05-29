package trader

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

// softExitReasons are the exhaust-driven book-decay exits. They are suppressed
// for config.MinExhaustHold after entry so a position is not flushed before it
// can clear its entry fee. Runway-expiry and stop exits are never suppressed.
func isSoftExitReason(reason string) bool {
	switch reason {
	case "book_thinning", "spread_widen",
		engine.ExitReasonPressureFade, engine.ExitReasonImbalanceFlip:
		return true
	default:
		return false
	}
}

func (crypto *Crypto) handleExit(exitSignal engine.Exit) error {
	if crypto.wallet == nil {
		return fmt.Errorf("wallet is required for exit")
	}

	if !engine.ValidExit(exitSignal) {
		return fmt.Errorf("invalid exit signal: %+v", exitSignal)
	}

	symbol := exitSignal.Symbol

	if !crypto.holdsSymbol(crypto.wallet, symbol) {
		audit("trade_exit_skip", map[string]any{
			"symbol":  symbol,
			"reason":  "no_position",
			"urgency": exitSignal.Urgency,
		})

		crypto.forecasts.ClearStop(symbol)
		delete(crypto.pumpPeak, symbol)

		return nil
	}

	if isSoftExitReason(exitSignal.Reason) {
		if binding, ok := crypto.wallet.PositionBindingFor(symbolBase(symbol)); ok {
			// Pump positions are never time-blocked from exiting (§15.3);
			// their downside is bounded by the trailing stop instead.
			if !isPumpRegime(binding.Regime) &&
				time.Since(binding.PredictedAt) < config.System.MinExhaustHold {
				audit("trade_exit_skip", map[string]any{
					"symbol":  symbol,
					"reason":  "min_hold",
					"signal":  exitSignal.Reason,
					"held_ms": time.Since(binding.PredictedAt).Milliseconds(),
				})

				return nil
			}
		}
	}

	// Source bid/ask/last from the price cache directly so the sell pays
	// the same half-spread the corresponding entry paid. Falling back to a
	// flat last on both sides would silently leak the spread into realized
	// PnL (and from there into the Kelly calibrator) — only the entry side
	// would see friction, and exits would look better than reality.
	last, bid, ask, eventAt, ok := crypto.forecasts.LastQuote(symbol)

	if !ok || last <= 0 {
		fallback := crypto.wallet.AvgEntryFor(symbolBase(symbol))

		if fallback <= 0 {
			audit("trade_exit_skip", map[string]any{
				"symbol":  symbol,
				"reason":  "missing_quote",
				"urgency": exitSignal.Urgency,
			})

			return fmt.Errorf("no quote for exit: %s", symbol)
		}

		last = fallback
		bid = fallback
		ask = fallback
	}

	if bid <= 0 {
		bid = last
	}

	if ask <= 0 {
		ask = last
	}

	if exitSignal.Reason == engine.ExitReasonStopHit && exitSignal.LimitPrice > 0 {
		// A stop never credits a price above its trigger. Take the worse of
		// the trigger and the current quote so paper PnL does not flatter
		// stop-outs relative to live execution.
		if exitSignal.LimitPrice < last {
			last = exitSignal.LimitPrice
		}

		if exitSignal.LimitPrice < bid {
			bid = exitSignal.LimitPrice
		}

		if exitSignal.LimitPrice < ask {
			ask = exitSignal.LimitPrice
		}
	}

	audit("trade_exit_eval", map[string]any{
		"symbol":  symbol,
		"urgency": exitSignal.Urgency,
		"reason":  exitSignal.Reason,
		"mark":    last,
		"bid":     bid,
		"ask":     ask,
	})

	sell := broker.Sell{
		Symbol: symbol,
		Quote: broker.Quote{
			Last: last,
			Bid:  bid,
			Ask:  ask,
			At:   eventAt,
		},
	}

	// Snapshot the cost basis BEFORE the sell zeroes the slot. Sell.FillPaper
	// calls ZeroInventory, which deletes AvgEntry[base]; reading it after the
	// fill returns 0 and turns realized PnL into "exit price × qty" — a pure
	// revenue number with no cost. The risk account's daily PnL accumulator
	// would then look perpetually positive on every round-trip.
	avgEntryBefore := crypto.wallet.AvgEntryFor(symbolBase(symbol))

	fill, err := sell.FillPaper(crypto.wallet)

	if err != nil {
		audit("trade_exit_error", map[string]any{
			"symbol": symbol,
			"error":  err.Error(),
		})

		return errnie.Error(err)
	}

	if fill.Qty <= 0 {
		audit("trade_exit_skip", map[string]any{
			"symbol": symbol,
			"reason": "empty_fill",
		})

		return nil
	}

	exitNotional := fill.Qty * fill.Price
	exitFee := exitNotional * crypto.wallet.FeePct / 100
	realizedNet := (fill.Price-avgEntryBefore)*fill.Qty - exitFee
	realizedReturnNet := 0.0

	if avgEntryBefore > 0 {
		realizedReturnNet = (fill.Price-avgEntryBefore)/avgEntryBefore - crypto.wallet.FeePct/100
	}

	crypto.attachWalletMarks()
	crypto.forecasts.ClearStop(symbol)
	delete(crypto.pumpPeak, symbol)
	crypto.recordExitPnL(symbol, fill.Qty, fill.Price, avgEntryBefore)
	crypto.pool.CreateBroadcastGroup("executions", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: fill,
	})

	crypto.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":               "trade_exit",
		"ts":                  time.Now().UTC().Format(time.RFC3339Nano),
		"symbol":              symbol,
		"side":                fill.Side,
		"qty":                 fill.Qty,
		"price":               fill.Price,
		"reason":              exitSignal.Reason,
		"urgency":             exitSignal.Urgency,
		"realized_net_eur":    realizedNet,
		"realized_return_net": realizedReturnNet,
	}})

	audit("trade_exit_fill", map[string]any{
		"symbol":              symbol,
		"side":                fill.Side,
		"qty":                 fill.Qty,
		"price":               fill.Price,
		"avg_entry":           avgEntryBefore,
		"exit_notional_eur":   exitNotional,
		"exit_fee_eur":        exitFee,
		"realized_net_eur":    realizedNet,
		"realized_return_net": realizedReturnNet,
		"reason":              exitSignal.Reason,
		"urgency":             exitSignal.Urgency,
		"balance_eur":         crypto.wallet.BalanceCopy(),
		"reserved_eur":        crypto.wallet.ReservedCopy(),
		"open_count":          crypto.openCount(),
	})

	crypto.sendWallet()

	return nil
}
