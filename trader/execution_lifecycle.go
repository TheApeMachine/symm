package trader

import (
	"fmt"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/wallet"
)

type liveEntryState uint8

const (
	liveEntryResting liveEntryState = iota
	liveEntryCanceling
	liveEntryFallback
	liveEntryClosed
)

/*
executionManager owns live entry orders between submission, fills, cancels,
and optional taker fallback.
*/
type executionManager struct {
	crypto    *Crypto
	byClOrdID map[string]*liveEntryOrder
	byOrderID map[string]*liveEntryOrder
}

type liveEntryOrder struct {
	Symbol          string
	ClOrdID         string
	OrderID         string
	Notional        float64
	Reserved        float64
	FilledNotional  float64
	FilledQty       float64
	EntryConfidence float64
	Prediction      engine.Prediction
	PredictedReturn float64
	StopPrice       float64
	StopLimit       float64
	TakeProfit      float64
	HasLotDecimals  bool
	LotDecimals     int
	CreatedAt       time.Time
	ObservedTicks   int
	FallbackQueued  bool
	state           liveEntryState
}

func newExecutionManager(crypto *Crypto) *executionManager {
	return &executionManager{
		crypto:    crypto,
		byClOrdID: make(map[string]*liveEntryOrder),
		byOrderID: make(map[string]*liveEntryOrder),
	}
}

func (manager *executionManager) Track(entry liveEntryOrder) error {
	if entry.ClOrdID == "" {
		return fmt.Errorf("live entry cl_ord_id is required")
	}

	if entry.Symbol == "" || entry.Notional <= 0 || entry.Reserved <= 0 {
		return fmt.Errorf("invalid live entry")
	}

	entry.state = liveEntryResting
	manager.byClOrdID[entry.ClOrdID] = &entry

	if entry.OrderID != "" {
		manager.byOrderID[entry.OrderID] = manager.byClOrdID[entry.ClOrdID]
	}

	return nil
}

func (manager *executionManager) HandleAck(ack *order.Ack) {
	if manager == nil || ack == nil {
		return
	}

	switch ack.Method {
	case order.MethodAddOrder:
		manager.handleAddAck(ack)
	case order.MethodCancelOrder:
		manager.handleCancelAck(ack)
	}
}

func (manager *executionManager) HandleFill(fill order.Fill) {
	if manager == nil || fill.Side != "buy" {
		return
	}

	entry := manager.find(fill.ClOrdID, fill.OrderID)

	if entry == nil || entry.state == liveEntryClosed {
		return
	}

	cost := fill.Qty*fill.Price + fill.Fee
	entry.FilledQty += fill.Qty
	entry.FilledNotional += cost
	manager.settleReservation(entry, cost)
	manager.bindFilledPosition(entry)

	if entry.Reserved > config.System.MinCostEUR {
		return
	}

	manager.close(entry)
}

func (manager *executionManager) HandleExit(exitSignal engine.Exit) bool {
	if manager == nil || exitSignal.Symbol == "" {
		return false
	}

	handled := false

	for _, entry := range manager.byClOrdID {
		if entry.Symbol != exitSignal.Symbol || entry.state != liveEntryResting {
			continue
		}

		if exitSignal.Urgency < configuredExecutionCancelUrgency() {
			continue
		}

		if manager.cancel(entry, false) {
			handled = true
		}
	}

	return handled
}

func (manager *executionManager) ReviewMeasurement(measurement engine.Measurement) {
	if manager == nil || !robustShockSource(measurement.Source) {
		return
	}

	if len(measurement.Pairs) == 0 || measurement.Confidence <= 0 {
		return
	}

	symbol := measurement.Pairs[0].Wsname

	for _, entry := range manager.byClOrdID {
		if entry.Symbol != symbol || entry.state != liveEntryResting {
			continue
		}

		entry.ObservedTicks++

		if !entry.ShouldFallback(measurement) {
			continue
		}

		manager.cancel(entry, true)
	}
}

func (entry *liveEntryOrder) ShouldFallback(measurement engine.Measurement) bool {
	if entry == nil || entry.OrderID == "" || entry.Reserved < config.System.MinCostEUR {
		return false
	}

	if entry.ObservedTicks < configuredExecutionFallbackTicks() {
		return false
	}

	return measurement.Confidence >= entry.EntryConfidence
}

func (manager *executionManager) handleAddAck(ack *order.Ack) {
	entry := manager.find(ack.Result.ClOrdID, ack.Result.OrderID)

	if entry == nil {
		return
	}

	if !ack.Success {
		manager.releaseAndClose(entry)
		return
	}

	if ack.Result.OrderID == "" {
		return
	}

	entry.OrderID = ack.Result.OrderID
	manager.byOrderID[entry.OrderID] = entry
}

func (manager *executionManager) handleCancelAck(ack *order.Ack) {
	entry := manager.byOrderID[ack.Result.OrderID]

	if entry == nil {
		return
	}

	if !ack.Success {
		entry.state = liveEntryResting
		return
	}

	if entry.FallbackQueued {
		manager.submitFallback(entry)
		return
	}

	manager.releaseAndClose(entry)
}

func (manager *executionManager) find(clOrdID, orderID string) *liveEntryOrder {
	if clOrdID != "" {
		if entry := manager.byClOrdID[clOrdID]; entry != nil {
			return entry
		}
	}

	if orderID == "" {
		return nil
	}

	return manager.byOrderID[orderID]
}

func (manager *executionManager) cancel(entry *liveEntryOrder, fallback bool) bool {
	if entry == nil || entry.OrderID == "" {
		return false
	}

	entry.state = liveEntryCanceling
	entry.FallbackQueued = fallback
	manager.publishOrder(order.CancelOrder(entry.OrderID, ""))

	return true
}

func (manager *executionManager) submitFallback(entry *liveEntryOrder) {
	if entry == nil || entry.Reserved < config.System.MinCostEUR {
		manager.releaseAndClose(entry)
		return
	}

	clOrdID, err := order.NextClOrdID()

	if err != nil {
		manager.releaseAndClose(entry)
		return
	}

	delete(manager.byClOrdID, entry.ClOrdID)

	if entry.OrderID != "" {
		delete(manager.byOrderID, entry.OrderID)
	}

	entry.ClOrdID = clOrdID
	entry.OrderID = ""
	entry.state = liveEntryFallback
	manager.byClOrdID[clOrdID] = entry

	request := order.MarketBuyCash(entry.Symbol, entry.Reserved, entry.StopPrice, entry.StopLimit, "")
	request.Params.ClOrdID = clOrdID
	manager.publishOrder(request)
}

func (manager *executionManager) settleReservation(entry *liveEntryOrder, cost float64) {
	if cost <= 0 || entry.Reserved <= 0 || manager.crypto.wallet == nil {
		return
	}

	reserved := cost

	if reserved > entry.Reserved {
		reserved = entry.Reserved
	}

	if err := manager.crypto.wallet.SettleEntryReservation(reserved, cost); err != nil {
		audit("live_entry_reservation_error", map[string]any{
			"symbol": entry.Symbol,
			"error":  err.Error(),
		})

		return
	}

	entry.Reserved -= reserved
}

func (manager *executionManager) bindFilledPosition(entry *liveEntryOrder) {
	position := wallet.PositionBinding{
		Source:      engine.PerspectiveSource(entry.Prediction.Perspective.Type),
		PredictedAt: entry.Prediction.PredictedAt,
		DueAt:       entry.Prediction.DueAt,
	}

	if entry.HasLotDecimals {
		position.HasLotDecimals = true
		position.LotDecimals = entry.LotDecimals
	}

	manager.crypto.wallet.BindPosition(symbolBase(entry.Symbol), position)

	if entry.StopPrice > 0 {
		manager.crypto.forecasts.RegisterStop(entry.Symbol, entry.StopPrice)
	}

	if entry.TakeProfit > 0 {
		manager.crypto.forecasts.RegisterTakeProfit(entry.Symbol, entry.TakeProfit)
	}
}

func (manager *executionManager) releaseAndClose(entry *liveEntryOrder) {
	if entry == nil {
		return
	}

	if entry.Reserved > 0 && manager.crypto.wallet != nil {
		manager.crypto.wallet.ReleaseEntryReservation(entry.Reserved)
		entry.Reserved = 0
	}

	manager.close(entry)
}

func (manager *executionManager) close(entry *liveEntryOrder) {
	entry.state = liveEntryClosed
	delete(manager.byClOrdID, entry.ClOrdID)

	if entry.OrderID != "" {
		delete(manager.byOrderID, entry.OrderID)
	}
}

func (manager *executionManager) publishOrder(value any) {
	manager.crypto.broadcasts["orders"].Send(&qpool.QValue[any]{Value: value})
}
