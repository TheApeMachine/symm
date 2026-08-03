package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
)

type TradesHistory struct {
	Error  []any       `json:"error"`
	Result TradesHistoryResult `json:"result"`
}

type TradesHistoryResult struct {
	Trades map[string]spot.Trade `json:"trades"`
}

func (history *TradesHistory) Action() string {
	return "TradesHistory"
}

func (history *TradesHistory) IsSuccess() bool {
	return len(history.Error) == 0
}

type TradesHistoryRequest struct {
	Type             string `json:"type"`
	Trades           bool   `json:"trades"`
	ConsolidateTaker bool   `json:"consolidate_taker"`
	WithoutCount     bool   `json:"without_count"`
	Ledgers          bool   `json:"ledgers"`
	RebaseMultiplier string `json:"rebase_multiplier"`
}

func (request *TradesHistoryRequest) MarshalJSON() ([]byte, error) {
	type alias TradesHistoryRequest
	return sonic.Marshal((*alias)(request))
}

func NewTradesHistoryFromMap(model datura.Map[any]) *TradesHistory {
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

	return &TradesHistory{
		Result: TradesHistoryResult{
			Trades: trades,
		},
	}
}
