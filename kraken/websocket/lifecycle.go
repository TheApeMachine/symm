package websocket

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
)

/*
Lifecycle emits paper order and execution frames through the latency simulator.
*/
type Lifecycle struct {
	paper *Paper
}

func NewLifecycle(paper *Paper) *Lifecycle {
	return &Lifecycle{paper: paper}
}

func (lifecycle *Lifecycle) Balance() error {
	var model datura.Map[any]
	var err error

	lifecycle.paper.simulator.Do(REST, func() {
		model, err = lifecycle.paper.execute("balances", "balance")
	})

	if err != nil {
		return err
	}

	return lifecycle.paper.simulator.Emit(
		lifecycle.paper, WEBSOCKET, "balances", kraken.NewBalanceFromMap(model),
	)
}

func (lifecycle *Lifecycle) Replay(trades []any) error {
	for tradeIndex, tradeRaw := range trades {
		trade, ok := tradeRaw.(map[string]any)

		if !ok {
			continue
		}

		execution := kraken.NewExecutionFromMap(datura.Map[any](trade))

		if tradeIndex == 0 {
			execution.Type = "snapshot"
		}

		err := lifecycle.paper.simulator.Emit(lifecycle.paper, WEBSOCKET, "executions", execution)

		if err != nil {
			return err
		}
	}

	return nil
}

func (lifecycle *Lifecycle) Place(model datura.Map[any], reqID int) error {
	orderAck := kraken.NewOrderResponseFromMap(model, reqID)

	err := lifecycle.paper.simulator.Emit(lifecycle.paper, WEBSOCKET, "add_order", orderAck)

	if err != nil {
		return err
	}

	action, _ := model["action"].(string)

	if action == "limit_order_placed" {
		err = lifecycle.paper.simulator.Emit(
			lifecycle.paper, WEBSOCKET, "executions", kraken.NewExecutionFromMap(model),
		)

		if err != nil {
			return err
		}

		return lifecycle.Balance()
	}

	executions := lifecycle.fills(model)

	for legIndex, execution := range executions {
		if legIndex > 0 {
			lifecycle.paper.simulator.Do(FILL, func() {})
		}

		err = lifecycle.paper.simulator.Emit(lifecycle.paper, WEBSOCKET, "executions", execution)

		if err != nil {
			return err
		}
	}

	return lifecycle.Balance()
}

func (lifecycle *Lifecycle) fills(model datura.Map[any]) []*kraken.Execution {
	volumeRaw, _ := model["volume"].(float64)
	volume := decimal.NewFromFloat64(volumeRaw)

	if volume.Sign() <= 0 {
		return []*kraken.Execution{kraken.NewExecutionFromMap(model)}
	}

	firstQty := volume.Div(decimal.NewFromInt64(2))
	secondQty := volume.Sub(firstQty)

	if firstQty.Sign() <= 0 || secondQty.Sign() <= 0 {
		return []*kraken.Execution{kraken.NewExecutionFromMap(model)}
	}

	orderID, _ := model["order_id"].(string)
	tradeID, _ := model["trade_id"].(string)
	pair, _ := model["pair"].(string)
	side, _ := model["side"].(string)
	priceRaw, _ := model["price"].(float64)
	costRaw, _ := model["cost"].(float64)
	lastPrice := decimal.NewFromFloat64(priceRaw)
	totalCost := decimal.NewFromFloat64(costRaw)
	timestamp := time.Now()

	firstCost := totalCost.Mul(firstQty).Div(volume)
	secondCost := totalCost.Sub(firstCost)

	return []*kraken.Execution{
		lifecycle.trade(
			orderID, tradeID+"-1", pair, side,
			firstQty, firstCost, firstQty, firstCost, lastPrice,
			"partially_filled", timestamp,
		),
		lifecycle.trade(
			orderID, tradeID+"-2", pair, side,
			secondQty, secondCost, volume, totalCost, lastPrice,
			"filled", timestamp,
		),
	}
}

func (lifecycle *Lifecycle) trade(
	orderID string,
	execID string,
	pair string,
	side string,
	lastQty *decimal.Decimal,
	cost *decimal.Decimal,
	cumQty *decimal.Decimal,
	cumCost *decimal.Decimal,
	lastPrice *decimal.Decimal,
	orderStatus string,
	timestamp time.Time,
) *kraken.Execution {
	return &kraken.Execution{
		Channel: "executions",
		Type:    "update",
		Data: []kraken.ExecutionData{{
			OrderID:     orderID,
			ExecID:      execID,
			ExecType:    "trade",
			Symbol:      pair,
			Side:        side,
			LastQty:     lastQty.Float64(),
			LastPrice:   *lastPrice,
			Cost:        *cost,
			OrderStatus: orderStatus,
			CumQty:      cumQty.Float64(),
			CumCost:     *cumCost,
			AvgPrice:    *lastPrice,
			Timestamp:   timestamp,
		}},
	}
}
