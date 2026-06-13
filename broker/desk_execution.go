package broker

import (
	"strings"
	"sync"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
)

func (desk *Desk) executionFillDelta(
	orderKey string,
	execution user.Execution,
	action *logic.Action,
) float64 {
	if desk == nil || action == nil || orderKey == "" {
		return 0
	}

	desk.ensureFillMap()

	if execution.CumQty > 0 {
		previous := desk.filledQuantity(orderKey)

		if execution.CumQty <= previous {
			return 0
		}

		desk.fills.Store(orderKey, execution.CumQty)

		return execution.CumQty - previous
	}

	if execution.LastQty > 0 {
		previous := desk.filledQuantity(orderKey)
		desk.fills.Store(orderKey, previous+execution.LastQty)

		return execution.LastQty
	}

	if execution.OrderStatus != "filled" || action.Quantity <= 0 {
		return 0
	}

	previous := desk.filledQuantity(orderKey)

	if action.Quantity <= previous {
		return 0
	}

	desk.fills.Store(orderKey, action.Quantity)

	return action.Quantity - previous
}

func (desk *Desk) filledQuantity(orderKey string) float64 {
	if desk == nil || desk.fills == nil {
		return 0
	}

	raw, ok := desk.fills.Load(orderKey)

	if !ok {
		return 0
	}

	quantity, ok := raw.(float64)

	if !ok || quantity <= 0 {
		return 0
	}

	return quantity
}

func (desk *Desk) clearFilledQuantity(orderKey string) {
	if desk == nil || desk.fills == nil || orderKey == "" {
		return
	}

	desk.fills.Delete(orderKey)
}

func (desk *Desk) ensureFillMap() {
	if desk.fills != nil {
		return
	}

	desk.fills = &sync.Map{}
}

func (desk *Desk) entryStop(
	symbol string,
	fillQty float64,
	fillPrice float64,
) (*StopLoss, error) {
	raw, ok := desk.stops.Load(symbol)

	if !ok {
		return NewStopLoss(symbol, fillQty, fillPrice, 0)
	}

	stopLoss, stopOK := raw.(*StopLoss)

	if !stopOK || stopLoss == nil || stopLoss.Quantity <= 0 {
		return NewStopLoss(symbol, fillQty, fillPrice, 0)
	}

	nextQuantity := stopLoss.Quantity + fillQty

	if nextQuantity <= 0 {
		return NewStopLoss(symbol, fillQty, fillPrice, 0)
	}

	stopLoss.EntryPrice = weightedEntryPrice(stopLoss, fillQty, fillPrice, nextQuantity)
	stopLoss.Quantity = nextQuantity
	stopLoss.State = StopArmed

	if fillPrice > stopLoss.PeakPrice {
		stopLoss.PeakPrice = fillPrice
		trailStop := stopLoss.PeakPrice * (1 - stopLoss.Offset)
		stopLoss.StopPrice = effectiveStopPrice(stopLoss.EntryPrice, trailStop, stopLoss.HardStopPrice)
	}

	return stopLoss, nil
}

func economicEntryPrice(
	execution user.Execution,
	fillQty float64,
	fillPrice float64,
) float64 {
	if fillQty <= 0 || fillPrice <= 0 {
		return fillPrice
	}

	if execution.FeeUsdEquiv <= 0 {
		return fillPrice
	}

	if execution.Side != string(trading.Buy) {
		return fillPrice
	}

	return fillPrice + (execution.FeeUsdEquiv / fillQty)
}

func weightedEntryPrice(
	stopLoss *StopLoss,
	fillQty float64,
	fillPrice float64,
	nextQuantity float64,
) float64 {
	existingCost := stopLoss.EntryPrice * stopLoss.Quantity
	fillCost := fillPrice * fillQty

	return (existingCost + fillCost) / nextQuantity
}

func (desk *Desk) applyStopExitFill(symbol string, fillQty float64) {
	if desk == nil || symbol == "" || fillQty <= 0 {
		return
	}

	raw, ok := desk.stops.Load(symbol)

	if !ok {
		return
	}

	stopLoss, stopOK := raw.(*StopLoss)

	if !stopOK || stopLoss == nil {
		return
	}

	desk.recordStopExitFilled(symbol, stopLoss.TriggeredAt)

	if stopLoss.Reduce(fillQty) {
		desk.stops.Delete(symbol)
	}
}

func (desk *Desk) markStopExitSubmitted(action *logic.Action) {
	if !isExitSubmission(action) {
		return
	}

	raw, ok := desk.stops.Load(action.Symbol)

	if !ok {
		return
	}

	stopLoss, stopOK := raw.(*StopLoss)

	if !stopOK || stopLoss == nil {
		return
	}

	stopLoss.MarkExitSubmitted()
}

func (desk *Desk) markStopNeedsRepair(action *logic.Action) {
	if !isExitSubmission(action) {
		return
	}

	raw, ok := desk.stops.Load(action.Symbol)

	if !ok {
		return
	}

	stopLoss, stopOK := raw.(*StopLoss)

	if !stopOK || stopLoss == nil {
		return
	}

	stopLoss.MarkNeedsRepair()
	desk.recordStopNeedsRepair(action.Symbol, "execution rejected")
}

func isExitSubmission(action *logic.Action) bool {
	if action == nil || action.Symbol == "" {
		return false
	}

	return action.Type.IsExit() || action.Side == trading.Sell
}

func isTerminalExecutionStatus(status string) bool {
	switch normalizedExecutionStatus(status) {
	case "filled", "canceled", "cancelled", "expired", "rejected":
		return true
	default:
		return false
	}
}

func isRejectedExecutionStatus(status string) bool {
	switch normalizedExecutionStatus(status) {
	case "canceled", "cancelled", "expired", "rejected":
		return true
	default:
		return false
	}
}

func normalizedExecutionStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}
