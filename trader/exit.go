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
			"symbol": symbol,
			"reason": "no_position",
			"urgency": exitSignal.Urgency,
		})

		return nil
	}

	last := crypto.forecasts.LastPrice(symbol)

	if last <= 0 {
		base := symbolBase(symbol)
		last = crypto.wallet.AvgEntry[base]
	}

	if last <= 0 {
		audit("trade_exit_skip", map[string]any{
			"symbol": symbol,
			"reason": "missing_quote",
			"urgency": exitSignal.Urgency,
		})

	audit("trade_exit_eval", map[string]any{
		"urgency": exitSignal.Urgency,
		"reason":  exitSignal.Reason,
		"mark":    last,
	})

	sell := broker.Sell{
		Symbol: symbol,
		Quote: broker.Quote{
			Last: last,
			Bid:  last,
			Ask:  last,
		},
	}

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
		"symbol":  symbol,
		"side":    fill.Side,
		"qty":     fill.Qty,
		"price":   fill.Price,
		"reason":  exitSignal.Reason,
		"urgency": exitSignal.Urgency,
		"balance_eur":  crypto.wallet.Balance,
		"reserved_eur": crypto.wallet.ReservedEUR,
		"open_count":   crypto.openCount(),
	})

	crypto.sendWallet()

	return nil
}
