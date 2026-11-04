package kraken

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
)

type Execution struct {
	Channel  string          `json:"channel"`
	Type     string          `json:"type"`
	Data     []ExecutionData `json:"data"`
	Sequence int             `json:"sequence"`
	onChange func(market.MarketType)
}

type ExecutionData struct {
	OrderID       string           `json:"order_id"`
	ClientOrderID string           `json:"cl_ord_id"`
	OrderUserref  int              `json:"order_userref"`
	ExecID        string           `json:"exec_id"`
	ExecType      string           `json:"exec_type"`
	TradeID       int              `json:"trade_id"`
	Symbol        string           `json:"symbol"`
	Side          string           `json:"side"`
	LastQty       *decimal.Decimal `json:"last_qty"`
	LastPrice     *decimal.Decimal `json:"last_price"`
	LiquidityInd  string           `json:"liquidity_ind"`
	Cost          *decimal.Decimal `json:"cost"`
	OrderType     string           `json:"order_type"`
	Timestamp     time.Time        `json:"timestamp"`
	OrderStatus   string           `json:"order_status"`
	CumQty        *decimal.Decimal `json:"cum_qty"`
	CumCost       *decimal.Decimal `json:"cum_cost"`
	AvgPrice      *decimal.Decimal `json:"avg_price"`
	FeeUsdEquiv   *decimal.Decimal `json:"fee_usd_equiv"`
	Fees          []ExecutionFee   `json:"fees"`
}

type ExecutionFee struct {
	Asset string  `json:"asset"`
	Qty   float64 `json:"qty"`
}

func NewExecution() *Execution {
	return &Execution{
		Channel: "executions",
		Type:    "update",
		Data:    make([]ExecutionData, 0),
	}
}

/*
NewExecutionFromMap builds a wire Execution from a paper CLI model.
*/
func NewExecutionFromMap(model map[string]any) json.Marshaler {
	if _, ok := model["trades"]; ok {
		return executionFromHistory(model)
	}

	return executionFromEvent(model)
}

func executionFromHistory(model map[string]any) *Execution {
	history, _ := NewTradesHistoryFromMap(model).(*TradesHistoryResult)
	data := make([]ExecutionData, 0)

	if history != nil {
		for id, trade := range history.Trades {
			data = append(data, ExecutionData{
				OrderID:     trade.OrderID,
				ExecID:      id,
				ExecType:    "trade",
				Symbol:      trade.Pair,
				Side:        trade.Type,
				LastQty:     trade.Volume,
				LastPrice:   trade.Price,
				Cost:        trade.Cost,
				Timestamp:   trade.Time,
				OrderStatus: "filled",
				CumQty:      trade.Volume,
				CumCost:     trade.Cost,
				AvgPrice:    trade.Price,
				FeeUsdEquiv: trade.Fee,
			})
		}
	}

	slices.SortFunc(data, func(a, b ExecutionData) int {
		if c := a.Timestamp.Compare(b.Timestamp); c != 0 {
			return c
		}

		if a.ExecID < b.ExecID {
			return -1
		}

		if a.ExecID > b.ExecID {
			return 1
		}

		return 0
	})

	return &Execution{
		Channel: "executions",
		Type:    "snapshot",
		Data:    data,
	}
}

func executionFromEvent(model map[string]any) *Execution {
	var execType string
	var orderStatus string

	if et, ok := model["exec_type"].(string); ok {
		execType = et
	}

	if os, ok := model["order_status"].(string); ok {
		orderStatus = os
	}

	if status, ok := model["status"].(string); ok && orderStatus == "" {
		orderStatus = status
	}

	if action, ok := model["action"].(string); ok {
		if action == "limit_order_placed" {
			if execType == "" {
				execType = "new"
			}
			if orderStatus == "" {
				orderStatus = "open"
			}
		}

		if action == "order_cancelled" {
			if execType == "" {
				execType = "canceled"
			}
			if orderStatus == "" {
				orderStatus = "canceled"
			}
		}

		if action == "market_order_filled" {
			if execType == "" {
				execType = "trade"
			}
			if orderStatus == "" {
				orderStatus = "filled"
			}
		}
	}

	orderID, _ := model["order_id"].(string)
	clientOrderID, _ := model["cl_ord_id"].(string)
	execID, _ := model["trade_id"].(string)

	if execID == "" {
		execID, _ = model["id"].(string)
	}

	pair, _ := model["pair"].(string)
	side, _ := model["side"].(string)
	volume := decimal.Zero
	price := decimal.Zero
	cost := decimal.Zero
	fee := decimal.Zero

	if value, ok := model["volume"].(float64); ok {
		volume = decimal.NewFromFloat(value)
	}

	if value, ok := model["price"].(float64); ok {
		price = decimal.NewFromFloat(value)
	}

	if value, ok := model["cost"].(float64); ok {
		cost = decimal.NewFromFloat(value)
	}

	if value, ok := model["fee"].(float64); ok {
		fee = decimal.NewFromFloat(value)
	}

	var timestamp time.Time

	if timeRaw, ok := model["time"].(string); ok && timeRaw != "" {
		if parsed, err := time.Parse(time.RFC3339, timeRaw); err == nil {
			timestamp = parsed
		}

		if timestamp.IsZero() {
			if parsed, err := time.Parse(time.RFC3339Nano, timeRaw); err == nil {
				timestamp = parsed
			}
		}
	}

	if timestamp.IsZero() {
		if sec, ok := model["timestamp"].(float64); ok && sec > 0 {
			timestamp = time.Unix(int64(sec), int64((sec-float64(int64(sec)))*1e9))
		}
	}

	return &Execution{
		Channel: "executions",
		Type:    "update",
		Data: []ExecutionData{{
			OrderID:       orderID,
			ClientOrderID: clientOrderID,
			ExecID:        execID,
			ExecType:      execType,
			Symbol:        pair,
			Side:          strings.ToLower(side),
			LastQty:       &volume,
			LastPrice:     &price,
			Cost:          &cost,
			OrderStatus:   orderStatus,
			CumQty:        &volume,
			CumCost:       &cost,
			AvgPrice:      &price,
			FeeUsdEquiv:   &fee,
			Timestamp:     timestamp,
		}},
	}
}

func (execution *Execution) Request() map[string]any {
	return map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":     "executions",
			"snap_orders": true,
			"snap_trades": true,
		},
	}
}

func (execution *Execution) OnChange(fn func(market.MarketType)) {
	execution.onChange = fn
}

func (execution *Execution) MarshalJSON() ([]byte, error) {
	type alias Execution
	return sonic.Marshal((*alias)(execution))
}

func (execution *Execution) UnmarshalJSON(data []byte) error {
	type alias Execution
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	onChange := execution.onChange
	*execution = Execution(decoded)
	execution.onChange = onChange

	if execution.Data == nil {
		execution.Data = make([]ExecutionData, 0)
	}

	if execution.onChange != nil {
		execution.onChange(execution)
	}

	return nil
}

func (execution *Execution) ToMarket(other market.MarketType) market.MarketType {
	book, ok := other.(*market.Execution)

	if !ok || book == nil || execution == nil {
		return other
	}

	rows := make([]*market.ExecutionData, 0, len(execution.Data))

	for _, row := range execution.Data {
		rows = append(rows, &market.ExecutionData{
			OrderID:       row.OrderID,
			ClientOrderID: row.ClientOrderID,
			OrderUserref:  row.OrderUserref,
			ExecID:        row.ExecID,
			ExecType:      row.ExecType,
			TradeID:       row.TradeID,
			Symbol:        row.Symbol,
			Side:          row.Side,
			LastQty:       row.LastQty,
			LastPrice:     row.LastPrice,
			LiquidityInd:  row.LiquidityInd,
			Cost:          row.Cost,
			OrderType:     row.OrderType,
			Timestamp:     row.Timestamp,
			OrderStatus:   row.OrderStatus,
			CumQty:        row.CumQty,
			CumCost:       row.CumCost,
			AvgPrice:      row.AvgPrice,
			FeeUsdEquiv:   row.FeeUsdEquiv,
		})
	}

	return market.NewExecution(map[string]any{
		"channel": execution.Channel,
		"type":    execution.Type,
		"data":    rows,
	}).ToMarket(book)
}

func (execution *Execution) FromMarket(other market.MarketType) market.MarketType {
	book, ok := other.(*market.Execution)

	if !ok || book == nil || execution == nil {
		return execution
	}

	execution.Channel = book.Channel
	execution.Type = book.Type
	execution.Sequence = book.Sequence
	execution.Data = execution.Data[:0]

	for _, row := range book.Data {
		if row == nil {
			continue
		}

		execution.Data = append(execution.Data, ExecutionData{
			OrderID:       row.OrderID,
			ClientOrderID: row.ClientOrderID,
			OrderUserref:  row.OrderUserref,
			ExecID:        row.ExecID,
			ExecType:      row.ExecType,
			TradeID:       row.TradeID,
			Symbol:        row.Symbol,
			Side:          row.Side,
			LastQty:       row.LastQty,
			LastPrice:     row.LastPrice,
			LiquidityInd:  row.LiquidityInd,
			Cost:          row.Cost,
			OrderType:     row.OrderType,
			Timestamp:     row.Timestamp,
			OrderStatus:   row.OrderStatus,
			CumQty:        row.CumQty,
			CumCost:       row.CumCost,
			AvgPrice:      row.AvgPrice,
			FeeUsdEquiv:   row.FeeUsdEquiv,
		})
	}

	return execution
}

/*
Tradable reports a fill the desk can fold into a lot.
*/
func (execution *Execution) Tradable() bool {
	if execution == nil {
		return false
	}

	for _, row := range execution.Data {
		if row.LastQty == nil || row.LastQty.Sign() <= 0 {
			continue
		}

		if row.ExecType == "" || row.ExecType == "trade" {
			return true
		}
	}

	return false
}

/*
DecodeExecution decodes an executions frame. An undecodable frame is an error:
applying a partially decoded fill would silently diverge the desk's positions.
*/
func DecodeExecution(buf []byte) (*Execution, error) {
	frame := NewExecution()

	if err := sonic.Unmarshal(buf, frame); err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[kraken.execution] invalid executions frame",
			err,
		)
	}

	return frame, nil
}
