package kraken

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
)

type HistoryTrade struct {
	OrderID string           `json:"ordertxid"`
	Pair    string           `json:"pair"`
	Type    string           `json:"type"`
	Price   *decimal.Decimal `json:"price"`
	Cost    *decimal.Decimal `json:"cost"`
	Fee     *decimal.Decimal `json:"fee"`
	Volume  *decimal.Decimal `json:"vol"`
	Time    time.Time        `json:"time"`
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
		price := decimal.Zero
		cost := decimal.Zero
		fee := decimal.Zero
		volume := decimal.Zero

		if value, ok := trade["price"].(float64); ok {
			price = decimal.NewFromFloat(value)
		}

		if value, ok := trade["cost"].(float64); ok {
			cost = decimal.NewFromFloat(value)
		}

		if value, ok := trade["fee"].(float64); ok {
			fee = decimal.NewFromFloat(value)
		}

		if value, ok := trade["volume"].(float64); ok {
			volume = decimal.NewFromFloat(value)
		}

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
			Price:   &price,
			Cost:    &cost,
			Fee:     &fee,
			Volume:  &volume,
			Time:    timestamp,
		}
	}

	return &TradesHistoryResult{
		Count:  json.Number(fmt.Sprintf("%d", len(trades))),
		Trades: trades,
	}
}
