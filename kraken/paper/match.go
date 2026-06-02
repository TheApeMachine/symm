package paper

import (
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/trading"
)

/*
tryMatchQuote fills resting post-only limits when the live quote crosses them.
*/
func (orders *Orders) tryMatchQuote(symbol string, quote broker.Quote) {
	if symbol == "" {
		return
	}

	orders.mu.Lock()
	pending := make([]*openOrder, 0)

	for _, order := range orders.open {
		if order.symbol != symbol {
			continue
		}

		if !order.postOnly || order.orderType != trading.Limit {
			continue
		}

		if !restingLimitCrossed(order, quote) {
			continue
		}

		pending = append(pending, order)
	}

	orders.mu.Unlock()

	for _, order := range pending {
		orders.fillRestingOrder(order)
	}
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

func (orders *Orders) fillRestingOrder(order *openOrder) {
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
	}

	out := orders.fillParams(params)

	if out.Channel == "" {
		return
	}

	channel := orders.socket.broadcasts["raw"]

	if channel == nil {
		return
	}

	channel.Send(&qpool.QValue[any]{
		Type:  out.Channel,
		Value: out,
	})
}
