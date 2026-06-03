package paper

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
)

const (
	triggerSlippageBps = 0.5
)

func (orders *Orders) queueExecution(execution user.Execution) {
	orders.socket.scheduleExchangeDelivery(func() {
		orders.socket.broadcasts["kraken:private"].Send(&qpool.QValue[any]{
			Type:  public.OrdersChannel,
			Value: execution,
		})
	})
}

func (orders *Orders) queueExecutions(executions []user.Execution) {
	if len(executions) == 0 {
		return
	}

	for index := 1; index < len(executions); index++ {
		orders.queueExecution(executions[index])
	}
}

func (orders *Orders) executionMessage(execution user.Execution) map[string]any {
	data, err := sonic.Marshal([]user.Execution{execution})

	if err != nil {
		errnie.Error(err)

		return nil
	}

	return map[string]any{
		"channel": public.ExecutionsChannel,
		"type":    "update",
		"data":    json.RawMessage(data),
	}
}

func (orders *Orders) executionMessages(executions []user.Execution) map[string]any {
	if len(executions) == 0 {
		return nil
	}

	orders.queueExecutions(executions)

	return orders.executionMessage(executions[0])
}

func (orders *Orders) fillParams(params trading.AddParams) map[string]any {
	return orders.buildFillExecution(params)
}

func (orders *Orders) buildFillExecution(params trading.AddParams) map[string]any {
	clOrdID := params.ClOrdID

	if clOrdID == "" {
		clOrdID = orders.identifier.ClOrdID()
	}

	orderID := orders.identifier.OrderID()
	meta := orders.catalog.Meta(params.Symbol)
	isMaker := params.OrderType == trading.Limit
	side := string(params.Side)
	orderType := string(params.OrderType)

	fillPrice, slipBps := orders.resolveFillPrice(params)

	if fillPrice <= 0 {
		errnie.Debug("paper.Orders.fillParams: no reference price for", params.Symbol, orderType)

		return nil
	}

	if slipBps > 0 {
		factor := slipBps / 10_000.0

		if side == "buy" {
			fillPrice *= 1 + factor
		} else {
			fillPrice *= 1 - factor
		}
	}

	fillPrice = orders.roundToTick(fillPrice, meta.tickSize)
	cost := params.OrderQty * fillPrice

	feeRate := meta.takerPct / 100.0

	if isMaker {
		feeRate = meta.makerPct / 100.0
	}

	feeCost := orders.roundFee(cost * feeRate)
	liquidityInd := "t"

	if isMaker {
		liquidityInd = "m"
	}

	feeUSD := feeCost

	if meta.quote != "USD" && !strings.HasSuffix(meta.quote, "USD") {
		feeUSD = 0
	}

	execID := orders.identifier.ExecID()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	orders.balances.ApplyFill(params.Symbol, side, params.OrderQty, fillPrice, feeCost, execID)

	return orders.executionMessage(user.Execution{
		ExecType:     "trade",
		OrderID:      orderID,
		ClOrdID:      clOrdID,
		Symbol:       params.Symbol,
		Side:         side,
		OrderType:    orderType,
		OrderQty:     params.OrderQty,
		LastQty:      params.OrderQty,
		LastPrice:    fillPrice,
		AvgPrice:     fillPrice,
		CumQty:       params.OrderQty,
		CumCost:      cost,
		Cost:         cost,
		LiquidityInd: liquidityInd,
		OrderStatus:  "filled",
		Timestamp:    now,
		ExecID:       execID,
		FeeUsdEquiv:  feeUSD,
		FeeCcyPref:   meta.quote,
		Fees: []user.ExecutionFee{{
			Asset: meta.quote,
			Qty:   feeCost,
		}},
	})
}

func (orders *Orders) openExecution(order *openOrder) map[string]any {
	return orders.executionMessage(user.Execution{
		ExecType:    "new",
		OrderID:     order.orderID,
		ClOrdID:     order.clOrdID,
		Symbol:      order.symbol,
		Side:        string(order.side),
		OrderType:   string(order.orderType),
		OrderQty:    order.orderQty,
		LimitPrice:  order.limitPrice,
		OrderStatus: "open",
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (orders *Orders) cancelExecution(order *openOrder) user.Execution {
	return user.Execution{
		ExecType:    "canceled",
		OrderID:     order.orderID,
		ClOrdID:     order.clOrdID,
		Symbol:      order.symbol,
		Side:        string(order.side),
		OrderType:   string(order.orderType),
		OrderQty:    order.orderQty,
		LimitPrice:  order.limitPrice,
		OrderStatus: "canceled",
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (orders *Orders) fillRestingParams(
	params trading.AddParams,
	quote broker.Quote,
	trade market.TradeUpdate,
) map[string]any {
	clOrdID := params.ClOrdID

	if clOrdID == "" {
		clOrdID = orders.identifier.ClOrdID()
	}

	orderID := orders.identifier.OrderID()
	meta := orders.catalog.Meta(params.Symbol)
	side := string(params.Side)
	orderType := string(params.OrderType)

	fillPrice, _ := broker.MakerRestingFillPrice(
		params.Side, params.LimitPrice, quote, trade, meta.tickSize,
	)

	if fillPrice <= 0 {
		errnie.Debug("paper.Orders.fillRestingParams: no fill price for", params.Symbol)

		return nil
	}

	fillPrice = orders.roundToTick(fillPrice, meta.tickSize)
	cost := params.OrderQty * fillPrice
	feeRate := meta.makerPct / 100.0
	feeCost := orders.roundFee(cost * feeRate)

	feeUSD := feeCost

	if meta.quote != "USD" && !strings.HasSuffix(meta.quote, "USD") {
		feeUSD = 0
	}

	execID := orders.identifier.ExecID()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	orders.balances.ApplyFill(params.Symbol, side, params.OrderQty, fillPrice, feeCost, execID)

	return orders.executionMessage(user.Execution{
		ExecType:     "trade",
		OrderID:      orderID,
		ClOrdID:      clOrdID,
		Symbol:       params.Symbol,
		Side:         side,
		OrderType:    orderType,
		OrderQty:     params.OrderQty,
		LastQty:      params.OrderQty,
		LastPrice:    fillPrice,
		AvgPrice:     fillPrice,
		CumQty:       params.OrderQty,
		CumCost:      cost,
		Cost:         cost,
		LiquidityInd: "m",
		OrderStatus:  "filled",
		Timestamp:    now,
		ExecID:       execID,
		FeeUsdEquiv:  feeUSD,
		FeeCcyPref:   meta.quote,
		Fees: []user.ExecutionFee{{
			Asset: meta.quote,
			Qty:   feeCost,
		}},
	})
}

func (orders *Orders) resolveFillPrice(params trading.AddParams) (float64, float64) {
	switch params.OrderType {
	case trading.Limit:
		meta := orders.catalog.Meta(params.Symbol)
		quote, ok := orders.quotes.Snapshot(params.Symbol)

		if !ok {
			return params.LimitPrice, 0
		}

		trade := market.TradeUpdate{
			Symbol: params.Symbol,
			Price:  params.LimitPrice,
			Qty:    params.OrderQty,
		}

		return broker.MakerRestingFillPrice(
			params.Side, params.LimitPrice, quote, trade, meta.tickSize,
		)
	case trading.Market:
		quote, ok := orders.quotes.Snapshot(params.Symbol)

		if !ok {
			return 0, 0
		}

		fill, err := broker.SlippageFill(quote, params.Side, params.OrderQty)

		if err != nil {
			return 0, 0
		}

		return fill.Price, fill.SlippageBps
	default:
		fillPrice := 0.0

		if params.Triggers != nil {
			fillPrice = params.Triggers.Price
		}

		if fillPrice <= 0 {
			fillPrice = params.LimitPrice
		}

		return fillPrice, triggerSlippageBps
	}
}

func (orders *Orders) roundToTick(price, tick float64) float64 {
	if tick <= 0 {
		return price
	}

	return math.Round(price/tick) * tick
}

func (orders *Orders) roundFee(fee float64) float64 {
	return math.Round(fee*1e8) / 1e8
}

func (orders *Orders) restsOnBook(params trading.AddParams) bool {
	return params.OrderType == trading.Limit && params.PostOnly
}
