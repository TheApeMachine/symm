package response

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

type pendingTaker struct {
	params  trading.AddParams
	orderID string
	due     time.Time
}

func (orders *Orders) scheduleTaker(params trading.AddParams, orderID string) {
	latency := orders.sampleLatency()

	if latency <= 0 {
		orders.executeTaker(params, orderID, perspectives.ActionNone)
		return
	}

	orders.mu.Lock()
	orders.pendingTakers = append(orders.pendingTakers, pendingTaker{
		params:  params,
		orderID: orderID,
		due:     time.Now().Add(latency),
	})
	orders.mu.Unlock()
}

/*
CheckPending fires taker fills whose one-way latency has elapsed.
*/
func (orders *Orders) CheckPending() {
	now := time.Now()

	orders.mu.Lock()

	var ready []pendingTaker
	var remaining []pendingTaker

	for _, pending := range orders.pendingTakers {
		if !pending.due.After(now) {
			ready = append(ready, pending)
			continue
		}

		remaining = append(remaining, pending)
	}

	orders.pendingTakers = remaining
	orders.mu.Unlock()

	for _, pending := range ready {
		orders.executeTaker(
			pending.params,
			pending.orderID,
			perspectives.ActionNone,
		)
	}
}

func (orders *Orders) executeTaker(
	params trading.AddParams,
	orderID string,
	action perspectives.ActionType,
) {
	price, err := orders.takerFillPrice(params.Symbol, params.Side, params.OrderQty, 0, action)

	if err != nil {
		errnie.Error(err)
		orders.notifyFill(FillNotice{
			Params: params, OrderID: orderID, Reason: err.Error(),
		})

		return
	}

	fee := orders.feeAmount(params.Symbol, params.OrderQty, price, false)

	orders.notifyFill(FillNotice{
		Params:       params,
		OrderID:      orderID,
		Price:        price,
		Fee:          fee,
		LiquidityInd: "t",
		Maker:        false,
	})

	if params.Side == trading.Sell {
		orders.mu.Lock()
		delete(orders.triggers, params.Symbol)
		orders.mu.Unlock()
	}
}

func (orders *Orders) sampleLatency() time.Duration {
	if orders.latency == nil {
		return 0
	}

	return orders.latency.Next()
}
