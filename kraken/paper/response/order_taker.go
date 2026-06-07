package response

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

type pendingTaker struct {
	params  trading.AddParams
	orderID string
	due     time.Time
}

func (orders *Orders) scheduleTaker(params trading.AddParams, orderID string) {
	latency := orders.sampleLatency()

	if latency <= 0 {
		orders.executeTaker(params, orderID, reasoning.ActionNone)
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

	pendingCount := len(orders.pendingTakers)
	ready := make([]pendingTaker, 0, pendingCount)
	remaining := make([]pendingTaker, 0, pendingCount)

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
			reasoning.ActionNone,
		)
	}
}

func (orders *Orders) executeTaker(
	params trading.AddParams,
	orderID string,
	action reasoning.ActionType,
) {
	fill, err := orders.takerFillQuote(params.Symbol, params.Side, params.OrderQty, 0, action)

	if err != nil {
		errnie.Error(err)
		orders.notifyFill(FillNotice{
			Params: params, OrderID: orderID, Reason: err.Error(),
		})

		return
	}

	if fill.filledQty <= 0 {
		orders.notifyFill(FillNotice{
			Params: params, OrderID: orderID, Reason: "paper fill: no liquidity",
		})

		return
	}

	params.OrderQty = fill.filledQty
	fee := orders.feeAmount(params.Symbol, fill.filledQty, fill.price, false)

	orders.notifyFill(FillNotice{
		Params:        params,
		OrderID:       orderID,
		Price:         fill.price,
		Fee:           fee,
		LiquidityInd:  "t",
		Maker:         false,
		Partial:       fill.depthCoverage > 0 && fill.depthCoverage < 1,
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
