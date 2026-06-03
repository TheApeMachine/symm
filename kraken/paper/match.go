package paper

import (
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

/*
tryMatchQuote evaluates resting limits against the latest quote after network latency.
*/
func (orders *Orders) tryMatchQuote(symbol string, quote broker.Quote) {
	if symbol == "" {
		return
	}

	orders.mu.Lock()
	pending := make([]*openOrder, 0)

	for _, order := range orders.open {
		if order.symbol != symbol || !order.postOnly || order.orderType != trading.Limit {
			continue
		}

		if !orders.restingOrderLive(order, quote.UpdatedAt) {
			continue
		}

		if !restingLimitCrossed(order, quote) {
			continue
		}

		if !order.queue.Ready() {
			continue
		}

		pending = append(pending, order)
	}

	orders.mu.Unlock()

	for _, order := range pending {
		orders.fillRestingOrder(order, quote, market.TradeUpdate{
			Symbol:    symbol,
			Side:      restingTradeSide(order.side),
			Price:     order.limitPrice,
			Qty:       order.orderQty,
			Timestamp: quote.UpdatedAt,
		})
	}
}

func (orders *Orders) tryMatchTrade(_ string, trade market.TradeUpdate) {
	if trade.Symbol == "" {
		return
	}

	quote, ok := orders.quotes.Snapshot(trade.Symbol)

	if !ok {
		return
	}

	orders.mu.Lock()
	pending := make([]*openOrder, 0)

	for _, order := range orders.open {
		if order.symbol != trade.Symbol || !order.postOnly || order.orderType != trading.Limit {
			continue
		}

		if !orders.restingOrderLive(order, trade.Timestamp) {
			continue
		}

		depletionQty, relevant := broker.TradeDepletesMakerQueue(
			order.side, order.limitPrice, trade,
		)

		if !relevant {
			continue
		}

		order.queue.Deplete(depletionQty)

		if !order.queue.Ready() {
			continue
		}

		pending = append(pending, order)
	}

	orders.mu.Unlock()

	for _, order := range pending {
		orders.fillRestingOrder(order, quote, trade)
	}
}

func (orders *Orders) restingOrderLive(order *openOrder, eventAt time.Time) bool {
	if order.queue.ActiveAt <= 0 {
		return true
	}

	if eventAt.IsZero() {
		return time.Now().UnixNano() >= order.queue.ActiveAt
	}

	return eventAt.UnixNano() >= order.queue.ActiveAt
}

func restingLimitCrossed(order *openOrder, quote broker.Quote) bool {
	if order.side == trading.Buy && quote.Ask > 0 && order.limitPrice >= quote.Ask {
		return true
	}

	if order.side == trading.Sell && quote.Bid > 0 && order.limitPrice <= quote.Bid {
		return true
	}

	return false
}

func restingTradeSide(side trading.Side) string {
	if side == trading.Sell {
		return "buy"
	}

	return "sell"
}

func (orders *Orders) fillRestingOrder(
	order *openOrder,
	quote broker.Quote,
	trade market.TradeUpdate,
) {
	order, ok := orders.takeOrder(order.orderID)

	if !ok {
		return
	}

	params := trading.AddParams{
		OrderType:  order.orderType,
		Side:       order.side,
		Symbol:     order.symbol,
		OrderQty:   order.orderQty,
		LimitPrice: order.limitPrice,
		ClOrdID:    order.clOrdID,
		PostOnly:   true,
	}

	out := orders.fillRestingParams(params, quote, trade)

	if channel, _ := out["channel"].(string); channel == "" {
		return
	}

	orders.socket.deliverExecution(out)
}
