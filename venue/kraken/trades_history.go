package kraken

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
)

type HistoryTrade struct {
	OrderID   string           `json:"ordertxid"`
	Pair      string           `json:"pair"`
	Type      string           `json:"type"`
	Price     *decimal.Decimal `json:"price"`
	Cost      *decimal.Decimal `json:"cost"`
	Fee       *decimal.Decimal `json:"fee"`
	Volume    *decimal.Decimal `json:"vol"`
	Time      time.Time        `json:"time"`
}

type TradesHistoryResult struct {
	Count  json.Number             `json:"count"`
	Trades map[string]HistoryTrade `json:"trades"`
}

func (result *TradesHistoryResult) MarshalJSON() ([]byte, error) {
	type alias TradesHistoryResult
	return sonic.Marshal((*alias)(result))
}

func (result *TradesHistoryResult) UnmarshalJSON(data []byte) error {
	type alias TradesHistoryResult
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	*result = TradesHistoryResult(decoded)
	return nil
}

func NewTradesHistoryFromMap(model map[string]any) json.Marshaler {
	rawTrades, _ := model["trades"].([]any)
	trades := make(map[string]HistoryTrade, len(rawTrades))

	for _, rawTrade := range rawTrades {
		trade, ok := rawTrade.(map[string]any)

		if !ok {
			continue
		}

		id, _ := trade["id"].(string)
		orderID, _ := trade["order_id"].(string)
		pair, _ := trade["pair"].(string)
		side, _ := trade["side"].(string)
		price := dFloat(floatField(trade["price"]))
		cost := dFloat(floatField(trade["cost"]))
		fee := dFloat(floatField(trade["fee"]))
		volume := dFloat(floatField(trade["volume"]))
		timestamp := time.Now()

		if timeRaw, ok := trade["time"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, timeRaw); err == nil {
				timestamp = parsed
			}
		}

		trades[id] = HistoryTrade{
			OrderID: orderID,
			Pair:    pair,
			Type:    side,
			Price:   price,
			Cost:    cost,
			Fee:     fee,
			Volume:  volume,
			Time:    timestamp,
		}
	}

	return &TradesHistoryResult{
		Count:  json.Number(fmt.Sprintf("%d", len(trades))),
		Trades: trades,
	}
}

func NewExecutionsFromHistoryMap(model map[string]any) json.Marshaler {
	history, _ := NewTradesHistoryFromMap(model).(*TradesHistoryResult)
	data := make([]ExecutionData, 0)

	if history != nil {
		for id, trade := range history.Trades {
			data = append(data, ExecutionData{
				OrderID:       trade.OrderID,
				ExecID:        id,
				ExecType:      "trade",
				Symbol:        trade.Pair,
				Side:          trade.Type,
				LastQty:       trade.Volume,
				LastPrice:     trade.Price,
				Cost:          trade.Cost,
				Timestamp:     trade.Time,
				OrderStatus:   "filled",
				CumQty:        trade.Volume,
				CumCost:       trade.Cost,
				AvgPrice:      trade.Price,
				FeeUsdEquiv:   trade.Fee,
			})
		}
	}

	return &Execution{
		Channel: "executions",
		Type:    "snapshot",
		Data:    data,
	}
}
