package trader

import (
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/market/perspectives"
)

func (crypto *Crypto) handleAction(action perspectives.Action) error {
	if crypto.desk == nil || action.Symbol == "" {
		return nil
	}

	if crypto.desk.Halted() {
		activate.Once("trader/crypto:order-halted")
		return nil
	}

	if action.Side == trading.Buy {
		if crypto.streams != nil && crypto.streams.Has(action.Symbol) {
			return nil
		}

		if action.Quantity <= 0 {
			return nil
		}

		if _, loaded := crypto.pendingOrders.LoadOrStore(action.Symbol, struct{}{}); loaded {
			return nil
		}
	}

	if action.Side == trading.Sell {
		if crypto.streams != nil && !crypto.streams.Has(action.Symbol) {
			return nil
		}

		action = crypto.resolveSellQuantity(action)

		if action.Quantity <= 0 {
			return nil
		}
	}

	if err := crypto.desk.AddOrder(action); err != nil {
		if action.Side == trading.Buy {
			crypto.pendingOrders.Delete(action.Symbol)
		}

		if crypto.desk.Halted() {
			activate.Once("trader/crypto:order-halted")
			return nil
		}

		return fmt.Errorf("trader/crypto: order %s: %w", action.Symbol, err)
	}

	return nil
}

func (crypto *Crypto) resolveSellQuantity(action perspectives.Action) perspectives.Action {
	if action.Quantity > 0 {
		return action
	}

	baseAsset, _, found := strings.Cut(action.Symbol, "/")

	if !found || baseAsset == "" {
		return action
	}

	if quantity, ok := crypto.inventory[baseAsset]; ok && quantity > 0 {
		action.Quantity = quantity
	}

	return action
}

func (crypto *Crypto) publishFill(execution user.Execution) error {
	if execution.Symbol == "" || execution.LastQty <= 0 {
		return nil
	}

	activate.Once("trader/crypto:fill")

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	crypto.recordFillEconomics(execution)

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"OrderID": execution.OrderID,
		"Symbol":  execution.Symbol,
		"Side":    execution.Side,
		"Qty":     execution.LastQty,
		"Price":   execution.LastPrice,
	}})

	auditEvent := "entry"
	if execution.Side == "sell" {
		auditEvent = "exit"
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":       "audit",
		"ts":          ts,
		"audit_event": auditEvent,
		"seq":         crypto.auditSeq.Add(1),
		"symbol":      execution.Symbol,
		"source":      "trader",
		"reason":      execution.OrderID,
	}})

	return crypto.sendWallet(crypto.cash, walletInventorySnapshot(crypto.inventory))
}

func (crypto *Crypto) recordFillEconomics(execution user.Execution) {
	baseAsset, _, found := strings.Cut(execution.Symbol, "/")

	if !found || baseAsset == "" {
		return
	}

	crypto.marks[execution.Symbol] = execution.LastPrice

	if execution.Side == "buy" {
		held := crypto.inventory[baseAsset]
		previousAvg := crypto.avgEntry[baseAsset]
		totalQty := held + execution.LastQty

		if totalQty <= 0 {
			return
		}

		crypto.avgEntry[baseAsset] = (previousAvg*held + execution.LastPrice*execution.LastQty) / totalQty

		crypto.pendingOrders.Delete(execution.Symbol)

		if crypto.streams != nil {
			crypto.streams.Add(execution.Symbol)
		}

		return
	}

	remaining := crypto.inventory[baseAsset] - execution.LastQty

	if remaining > 0 {
		return
	}

	delete(crypto.avgEntry, baseAsset)

	crypto.pendingOrders.Delete(execution.Symbol)

	if crypto.streams == nil {
		return
	}

	crypto.streams.Remove(execution.Symbol)
}
