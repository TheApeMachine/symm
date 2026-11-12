package kraken

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
)

/*
HistoryTrade is one closed lot row in TradesHistory REST — broker reconciles
tier ledger against these ids when executions snapshots are incomplete.
*/
type HistoryTrade struct {
	OrderID    string           `json:"ordertxid"`
	PositionID string           `json:"postxid"`
	Pair       string           `json:"pair"`
	Type       string           `json:"type"`
	OrderType  string           `json:"ordertype"`
	Price      *decimal.Decimal `json:"price"`
	Cost       *decimal.Decimal `json:"cost"`
	Fee        *decimal.Decimal `json:"fee"`
	Volume     *decimal.Decimal `json:"vol"`
	Time       float64          `json:"time"`
}

/*
TradesHistory is private REST ledger sync — broker tier polls this to backfill
fills without trusting only the executions websocket.
*/
type TradesHistory struct {
	Error    []any             `json:"error"`
	Result   TradesHistoryResult `json:"result"`
	onChange func(market.MarketType)
}

/*
NewTradesHistory prepares an empty trades map before the first history poll.
*/
func NewTradesHistory() *TradesHistory {
	return &TradesHistory{
		Result: TradesHistoryResult{
			Trades: make(map[string]HistoryTrade),
		},
	}
}

/*
OnChange wires tier history ingest after each REST decode.
*/
func (history *TradesHistory) OnChange(fn func(market.MarketType)) {
	history.onChange = fn
}

/*
Request asks for all types without embedded trades — map is built from top-level trades.
*/
func (history *TradesHistory) Request() map[string]any {
	return map[string]any{
		"type":   "all",
		"trades": false,
	}
}

/*
UnmarshalJSON surfaces venue errors and fires OnChange after Trades is allocated.
*/
func (history *TradesHistory) UnmarshalJSON(data []byte) error {
	type alias TradesHistory
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	if len(decoded.Error) > 0 {
		return errnie.Err(
			errnie.IO, "[kraken.TradesHistory] venue error", nil,
		).With("error", decoded.Error)
	}

	onChange := history.onChange
	*history = TradesHistory(decoded)
	history.onChange = onChange

	if history.Result.Trades == nil {
		history.Result.Trades = make(map[string]HistoryTrade)
	}

	if history.onChange != nil {
		history.onChange(history)
	}

	return nil
}

/*
MarshalJSON aliases TradesHistory so onChange stays off cached REST payloads.
*/
func (history *TradesHistory) MarshalJSON() ([]byte, error) {
	type alias TradesHistory
	return sonic.Marshal((*alias)(history))
}

/*
ToMarket is identity — desk consumes HistoryTrade via broker, not nomagique projection.
*/
func (history *TradesHistory) ToMarket(other market.MarketType) market.MarketType {
	return other
}

/*
FromMarket is identity — history is venue-owned REST state.
*/
func (history *TradesHistory) FromMarket(other market.MarketType) market.MarketType {
	return history
}

/*
TradesHistoryResult is the result block with count, trades map, and cursor.
*/
type TradesHistoryResult struct {
	Count  json.Number             `json:"count"`
	Trades map[string]HistoryTrade `json:"trades"`
	Cursor TradesHistoryCursor     `json:"cursor"`
}

/*
TradesHistoryCursor is the cursor block when with_cursor is true on the request.
*/
type TradesHistoryCursor struct {
	Next string `json:"next"`
}

/*
MarshalJSON aliases TradesHistoryResult for sonic round-trips.
*/
func (result *TradesHistoryResult) MarshalJSON() ([]byte, error) {
	type alias TradesHistoryResult
	return sonic.Marshal((*alias)(result))
}

/*
UnmarshalJSON decodes TradesHistoryResult without running OnChange — nested in TradesHistory.
*/
func (result *TradesHistoryResult) UnmarshalJSON(data []byte) error {
	type alias TradesHistoryResult
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	*result = TradesHistoryResult(decoded)
	return nil
}

/*
NewTradesHistoryFromMap adapts paper CLI history into TradesHistoryResult for paper Post.
*/
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

		var when float64

		switch value := trade["time"].(type) {
		case float64:
			when = value
		case string:
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				when = float64(parsed.Unix())
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
			Time:    when,
		}
	}

	return &TradesHistoryResult{
		Count:  json.Number(fmt.Sprintf("%d", len(trades))),
		Trades: trades,
	}
}
