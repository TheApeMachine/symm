package response

import (
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

type restingMaker struct {
	params     trading.AddParams
	orderID    string
	limitPrice float64
	queue      broker.MakerQueueState
	tickSize   float64
}

func (orders *Orders) armMaker(params trading.AddParams, orderID string) {
	quote, ok := orders.quotes.Snapshot(params.Symbol)

	if !ok {
		orders.notifyFill(FillNotice{
			Params: params, OrderID: orderID,
			Reason: "paper fill: no quote for " + params.Symbol,
		})

		return
	}

	if orders.rejectCrossingPostOnly(params, orderID, quote) {
		return
	}

	latency := orders.sampleLatency()
	activeAt := time.Now().Add(latency).UnixNano()
	tickSize := orders.tickSize(params.Symbol)

	resting := restingMaker{
		params:     params,
		orderID:    orderID,
		limitPrice: params.LimitPrice,
		queue:      broker.NewMakerQueueState(quote, params.Side, params.LimitPrice, activeAt, tickSize),
		tickSize:   tickSize,
	}

	orders.mu.Lock()
	orders.makers[params.Symbol] = &resting
	orders.mu.Unlock()

	orders.notifyArm(params, orderID)
}

func (orders *Orders) onTrade(symbol string, trade market.TradeUpdate) {
	orders.mu.Lock()
	resting, ok := orders.makers[symbol]
	orders.mu.Unlock()

	if !ok || resting == nil {
		return
	}

	if time.Now().UnixNano() < resting.queue.ActiveAt {
		return
	}

	depletion, ok := broker.TradeDepletesMakerQueue(
		resting.params.Side,
		resting.limitPrice,
		trade,
	)

	if !ok {
		return
	}

	resting.queue.Deplete(depletion)

	if !resting.queue.Ready() {
		return
	}

	quote, quoteOK := orders.quotes.Snapshot(symbol)

	if !quoteOK {
		return
	}

	fillPrice, _ := broker.MakerRestingFillPrice(
		resting.params.Side,
		resting.limitPrice,
		quote,
		trade,
		resting.tickSize,
	)
	fee := orders.feeAmount(symbol, resting.params.OrderQty, fillPrice, true)

	orders.mu.Lock()
	delete(orders.makers, symbol)
	orders.mu.Unlock()

	orders.notifyFill(FillNotice{
		Params:       resting.params,
		OrderID:      resting.orderID,
		Price:        fillPrice,
		Fee:          fee,
		LiquidityInd: "m",
		Maker:        true,
	})
}
