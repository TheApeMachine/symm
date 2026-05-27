package trader

import (
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/order"
)

func (crypto *Crypto) handleExit(exitSignal engine.Exit) error {
	if crypto.wallet == nil {
		return errnie.Error(fmt.Errorf("wallet is required for exit"))
	}

	if !engine.ValidExit(exitSignal) {
		return errnie.Error(fmt.Errorf("invalid exit signal: %+v", exitSignal))
	}

	symbol := exitSignal.Symbol
	reason := exitSignal.Reason

	base := strings.Split(symbol, "/")[0]
	qty := crypto.wallet.Inventory[base]

	if qty <= config.System.LiveInventoryEpsilon {
		return nil
	}

	peakExit := exitSignal.Urgency >= config.System.ExitPeakUrgency &&
		(exitSignal.Reason == engine.ExitReasonImbalanceFlip ||
			exitSignal.Reason == engine.ExitReasonPressureFade)

	if crypto.wallet.Type == PaperWallet {
		return crypto.handlePaperExit(exitSignal, symbol, reason, base, qty, peakExit)
	}

	if peakExit {
		last := crypto.predictions.LastPrice(symbol)

		crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":   "peak_exit",
			"symbol":  symbol,
			"qty":     qty,
			"price":   last,
			"reason":  reason,
			"urgency": exitSignal.Urgency,
		}})
	}

	crypto.pool.CreateBroadcastGroup("orders", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: order.MarketSellBase(symbol, qty, ""),
	})

	return nil
}

func (crypto *Crypto) handlePaperExit(
	exitSignal engine.Exit,
	symbol, reason, base string,
	qty float64,
	peakExit bool,
) error {
	last := crypto.predictions.LastPrice(symbol)

	if last <= 0 {
		return errnie.Error(fmt.Errorf("no last price for paper exit: %s", symbol))
	}

	fillPrice := config.System.SlippageFill(
		last, last, last, "sell", config.System.SlippageBPS, qty*last, nil, nil,
	)

	if fillPrice <= 0 {
		return errnie.Error(fmt.Errorf("invalid fill price for paper exit: %s", symbol))
	}

	revenue := qty * fillPrice
	fee := revenue * crypto.wallet.FeePct / 100

	crypto.wallet.Inventory[base] = 0
	crypto.wallet.ClearPosition(base)
	crypto.wallet.Balance += revenue - fee

	crypto.pool.CreateBroadcastGroup("executions", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: order.Fill{
			OrderID: fmt.Sprintf("paper:exit:%s:%d", symbol, time.Now().UnixNano()),
			Symbol:  symbol,
			Side:    "sell",
			Qty:     qty,
			Price:   fillPrice,
		},
	})

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":   logicEvent(peakExit, "simulated_exit"),
		"symbol":  symbol,
		"qty":     qty,
		"price":   fillPrice,
		"reason":  reason,
		"urgency": exitSignal.Urgency,
	}})
	crypto.sendWallet()

	return nil
}

func logicEvent(peakExit bool, defaultEvent string) string {
	if peakExit {
		return "peak_exit"
	}

	return defaultEvent
}
