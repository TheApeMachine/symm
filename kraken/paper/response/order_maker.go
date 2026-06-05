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

	if !ok || resting == nil {
		orders.mu.Unlock()

		return
	}

	if time.Now().UnixNano() < resting.queue.ActiveAt {
		orders.mu.Unlock()

		return
	}

	depletion, depletes := broker.TradeDepletesMakerQueue(
		resting.params.Side,
		resting.limitPrice,
		trade,
	)

	if !depletes {
		orders.mu.Unlock()

		return
	}

	resting.queue.Deplete(depletion)

	if !resting.queue.Ready() {
		orders.mu.Unlock()

		return
	}

	delete(orders.makers, symbol)
	params := resting.params
	orderID := resting.orderID
	tickSize := resting.tickSize
	limitPrice := resting.limitPrice
	side := resting.params.Side
	qty := resting.params.OrderQty
	orders.mu.Unlock()

	quote, quoteOK := orders.quotes.Snapshot(symbol)

	if !quoteOK {
		return
	}

	fillPrice, _ := broker.MakerRestingFillPrice(
		side,
		limitPrice,
		quote,
		trade,
		tickSize,
	)

	if fillPrice <= 0 {
		return
	}

	fee := orders.feeAmount(symbol, qty, fillPrice, true)

	orders.notifyFill(FillNotice{
		Params:       params,
		OrderID:      orderID,
		Price:        fillPrice,
		Fee:          fee,
		LiquidityInd: "m",
		Maker:        true,
	})
}
