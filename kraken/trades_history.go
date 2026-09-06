package kraken

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
)

func NewTradesHistoryFromMap(model datura.Map[any]) spot.TradesHistoryResult {
	rawTrades, _ := model["trades"].([]any)
	trades := make(map[string]spot.Trade, len(rawTrades))

	for _, rawTrade := range rawTrades {
		trade, ok := rawTrade.(map[string]any)

		if !ok {
			continue
		}

		id, _ := trade["id"].(string)
		orderID, _ := trade["order_id"].(string)
		pair, _ := trade["pair"].(string)
		side, _ := trade["side"].(string)
		price, _ := trade["price"].(float64)
		cost, _ := trade["cost"].(float64)
		fee, _ := trade["fee"].(float64)
		volume, _ := trade["volume"].(float64)
		timestamp := time.Now()

		if timeRaw, ok := trade["time"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, timeRaw); err == nil {
				timestamp = parsed
			}
		}

		trades[id] = spot.Trade{
			OrderID:   orderID,
			Pair:      pair,
			Time:      decimal.NewFromFloat64(float64(timestamp.UnixNano()) / 1e9),
			Type:      side,
			OrderType: "market",
			Price:     decimal.NewFromFloat64(price),
			Cost:      decimal.NewFromFloat64(cost),
			Fee:       decimal.NewFromFloat64(fee),
			Volume:    decimal.NewFromFloat64(volume),
			Maker:     false,
		}
	}

	return spot.TradesHistoryResult{
		Count:  json.Number(fmt.Sprintf("%d", len(trades))),
		Trades: trades,
	}
}
