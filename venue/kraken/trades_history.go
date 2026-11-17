package kraken

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
)

type TradesHistoryRequest struct {
	Type         string `json:"type,omitempty"`
	Trades       bool   `json:"trades,omitempty"`
	Pair         string `json:"pair,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	WithoutCount bool   `json:"without_count,omitempty"`
	WithCursor   bool   `json:"with_cursor,omitempty"`
	Cursor       string `json:"cursor,omitempty"`
}

func NewTradesHistoryRequest(symbols []string) *TradesHistoryRequest {
	req := &TradesHistoryRequest{
		Type:         "all",
		Limit:        100,
		WithoutCount: true,
		WithCursor:   true,
	}

	if len(symbols) > 0 {
		req.Pair = strings.Join(symbols, ",")
	}

	return req
}

/*
HistoryTrade is one fill. The map key is the trade txid; ID is stamped from
that key because the object body only carries ordertxid.
*/
type HistoryTrade struct {
	ID         string           `json:"-"`
	OrderID    string           `json:"ordertxid"`
	PositionID string           `json:"postxid"`
	Pair       string           `json:"pair"`
	Type       string           `json:"type"`
	OrderType  string           `json:"ordertype"`
	Price      *decimal.Decimal `json:"price"`
	Cost       *decimal.Decimal `json:"cost"`
	Fee        *decimal.Decimal `json:"fee"`
	Volume     *decimal.Decimal `json:"vol"`
	Time       time.Time        `json:"time"`
}

/*
UnmarshalJSON accepts Kraken's unix-seconds float on time.
*/
func (trade *HistoryTrade) UnmarshalJSON(data []byte) error {
	type alias struct {
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

	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	*trade = HistoryTrade{
		OrderID:    decoded.OrderID,
		PositionID: decoded.PositionID,
		Pair:       decoded.Pair,
		Type:       decoded.Type,
		OrderType:  decoded.OrderType,
		Price:      decoded.Price,
		Cost:       decoded.Cost,
		Fee:        decoded.Fee,
		Volume:     decoded.Volume,
		Time:       time.Unix(int64(decoded.Time), 0).UTC(),
	}

	return nil
}

/*
TradesHistory is private REST ledger sync — broker tier polls this to backfill
fills without trusting only the executions websocket.
*/
type TradesHistory struct {
	Error  []any               `json:"error"`
	Result TradesHistoryResult `json:"result"`
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
Request asks for all types without embedded trades — map is built from top-level trades.
*/
func (history *TradesHistory) Request() map[string]any {
	return map[string]any{
		"type":          "all",
		"limit":         100,
		"without_count": true,
		"with_cursor":   true,
	}
}

/*
UnmarshalJSON surfaces venue errors after Trades is allocated.
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

	*history = TradesHistory(decoded)

	if history.Result.Trades == nil {
		history.Result.Trades = make(map[string]HistoryTrade)
	}

	return nil
}

/*
MarshalJSON aliases TradesHistory for cached REST payloads.
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

func NewTradesHistoryResult(data []byte) *TradesHistoryResult {
	var envelope TradesHistory

	if err := sonic.Unmarshal(data, &envelope); err != nil {
		errnie.Debug("[tradeshistory] unmarshal failed", "err", err, "bytes", len(data))
		return nil
	}

	if len(envelope.Error) > 0 {
		errnie.Debug("[tradeshistory] venue error", "error", envelope.Error)
		return nil
	}

	if envelope.Result.Trades == nil {
		envelope.Result.Trades = make(map[string]HistoryTrade)
	}

	errnie.Debug("[tradeshistory] parsed", "trades", len(envelope.Result.Trades), "count", envelope.Result.Count)
	return &envelope.Result
}

/*
Fills returns every trade with its map key stamped onto ID. Partial prints of
one order share ordertxid and are distinct only by that key.
*/
func (result *TradesHistoryResult) Fills() []HistoryTrade {
	if result == nil || len(result.Trades) == 0 {
		return nil
	}

	fills := make([]HistoryTrade, 0, len(result.Trades))

	for id, trade := range result.Trades {
		if trade.ID == "" {
			trade.ID = id
		}

		fills = append(fills, trade)
	}

	return fills
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
UnmarshalJSON decodes TradesHistoryResult nested in TradesHistory.
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

		var when time.Time

		switch value := trade["time"].(type) {
		case float64:
			when = time.Unix(int64(value), 0).UTC()
		case string:
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				when = parsed.UTC()
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
