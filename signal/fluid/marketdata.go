package fluid

import (
	"fmt"
	"time"

	"github.com/theapemachine/datura"
)

/*
BookLevel is one price/qty level in a book update payload.
*/
type BookLevel struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

/*
BookUpdate is the decoded book frame used by fluid symbol state.
*/
type BookUpdate struct {
	Symbol    string      `json:"symbol"`
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
}

/*
TickerUpdate is the decoded ticker frame used by fluid symbol state.
*/
type TickerUpdate struct {
	Symbol    string    `json:"symbol"`
	Last      float64   `json:"last"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	BidQty    float64   `json:"bid_qty"`
	AskQty    float64   `json:"ask_qty"`
	Change    float64   `json:"change"`
	ChangePct float64   `json:"change_pct"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Volume    float64   `json:"volume"`
	VWAP      float64   `json:"vwap"`
	Timestamp time.Time `json:"timestamp"`
}

/*
TradeUpdate is the decoded trade print used by fluid symbol state.
*/
type TradeUpdate struct {
	Symbol    string    `json:"symbol"`
	Side      string    `json:"side"`
	Price     float64   `json:"price"`
	Qty       float64   `json:"qty"`
	Timestamp time.Time `json:"timestamp"`
}

/*
eventTime resolves an event's wall-clock from the row's RFC3339 stamp or the
artifact timestamp. It never invents a wall-clock for market data.
*/
func eventTime(datapoint *datura.Artifact, rowIndex int) (time.Time, error) {
	stamp := rowString(datapoint, rowIndex, "timestamp")

	if stamp != "" {
		parsed, err := time.Parse(time.RFC3339Nano, stamp)

		if err != nil {
			return time.Time{}, fmt.Errorf("fluid: invalid event timestamp: %w", err)
		}

		return parsed.UTC(), nil
	}

	if datapoint != nil && datapoint.Timestamp() > 0 {
		return time.Unix(0, datapoint.Timestamp()).UTC(), nil
	}

	return time.Time{}, fmt.Errorf("fluid: event timestamp required")
}

func tickerUpdate(datapoint *datura.Artifact, rowIndex int, symbol string, eventAt time.Time) TickerUpdate {
	return TickerUpdate{
		Symbol:    symbol,
		Last:      rowFloat(datapoint, rowIndex, "last"),
		Bid:       rowFloat(datapoint, rowIndex, "bid"),
		Ask:       rowFloat(datapoint, rowIndex, "ask"),
		BidQty:    rowFloat(datapoint, rowIndex, "bid_qty"),
		AskQty:    rowFloat(datapoint, rowIndex, "ask_qty"),
		Change:    rowFloat(datapoint, rowIndex, "change"),
		ChangePct: rowFloat(datapoint, rowIndex, "change_pct"),
		High:      rowFloat(datapoint, rowIndex, "high"),
		Low:       rowFloat(datapoint, rowIndex, "low"),
		Volume:    rowFloat(datapoint, rowIndex, "volume"),
		Timestamp: eventAt,
	}
}

func bookUpdate(datapoint *datura.Artifact, rowIndex int, symbol string, eventAt time.Time) BookUpdate {
	updateType := rowString(datapoint, rowIndex, "type")

	if updateType == "" {
		updateType = datura.Peek[string](datapoint, "type")
	}

	return BookUpdate{
		Symbol:    symbol,
		Type:      updateType,
		Timestamp: eventAt,
		Bids:      bookLevels(datapoint, rowIndex, "bids"),
		Asks:      bookLevels(datapoint, rowIndex, "asks"),
	}
}

func bookLevels(datapoint *datura.Artifact, rowIndex int, side string) []BookLevel {
	levels := []BookLevel{}

	for levelIndex := 0; ; levelIndex++ {
		price := rowLevelFloat(datapoint, rowIndex, side, levelIndex, "price")

		if price <= 0 {
			return levels
		}

		levels = append(levels, BookLevel{
			Price: price,
			Qty:   rowLevelFloat(datapoint, rowIndex, side, levelIndex, "qty"),
		})
	}
}

func tradeUpdate(datapoint *datura.Artifact, rowIndex int, symbol string, eventAt time.Time) TradeUpdate {
	return TradeUpdate{
		Symbol:    symbol,
		Side:      rowString(datapoint, rowIndex, "side"),
		Price:     rowFloat(datapoint, rowIndex, "price"),
		Qty:       rowFloat(datapoint, rowIndex, "qty"),
		Timestamp: eventAt,
	}
}

func rowString(datapoint *datura.Artifact, rowIndex int, key string) string {
	if rowIndex < 0 {
		return datura.Peek[string](datapoint, key)
	}

	return datura.Peek[string](datapoint, "data", rowIndex, key)
}

func rowFloat(datapoint *datura.Artifact, rowIndex int, key string) float64 {
	if rowIndex < 0 {
		return datura.Peek[float64](datapoint, key)
	}

	return datura.Peek[float64](datapoint, "data", rowIndex, key)
}

func rowLevelFloat(datapoint *datura.Artifact, rowIndex int, side string, levelIndex int, key string) float64 {
	if rowIndex < 0 {
		return datura.Peek[float64](datapoint, side, levelIndex, key)
	}

	return datura.Peek[float64](datapoint, "data", rowIndex, side, levelIndex, key)
}
