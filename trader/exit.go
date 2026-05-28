package trader

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/engine"
)

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

		return nil
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

	crypto.attachWalletMarks()
	crypto.recordExitPnL(symbol, fill.Qty, fill.Price, avgEntryBefore)
	crypto.pool.CreateBroadcastGroup("executions", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: fill,
	})

	crypto.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":   "trade_exit",
		"ts":      time.Now().UTC().Format(time.RFC3339Nano),
		"symbol":  symbol,
		"side":    fill.Side,
		"qty":     fill.Qty,
		"price":   fill.Price,
		"reason":  exitSignal.Reason,
		"urgency": exitSignal.Urgency,
	}})

	audit("trade_exit_fill", map[string]any{
		"symbol":       symbol,
		"side":         fill.Side,
		"qty":          fill.Qty,
		"price":        fill.Price,
		"reason":       exitSignal.Reason,
		"urgency":      exitSignal.Urgency,
		"balance_eur":  crypto.wallet.BalanceCopy(),
		"reserved_eur": crypto.wallet.ReservedCopy(),
		"open_count":   crypto.openCount(),
	})

	crypto.sendWallet()

	return nil
}
