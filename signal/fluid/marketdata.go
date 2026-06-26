package fluid

import (
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
eventTime resolves an event's wall-clock from the row's RFC3339 stamp, falling
back to the artifact timestamp and finally to now.
*/
func eventTime(datapoint *datura.Artifact, rowIndex int) time.Time {
	stamp := datura.Peek[string](datapoint, "data", rowIndex, "timestamp")

	if stamp != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			return parsed.UTC()
		}
	}

	if datapoint != nil && datapoint.Timestamp() > 0 {
		return time.Unix(0, datapoint.Timestamp()).UTC()
	}

	return time.Now().UTC()
}

func tickerUpdate(datapoint *datura.Artifact, rowIndex int, symbol string) TickerUpdate {
	return TickerUpdate{
		Symbol:    symbol,
		Last:      datura.Peek[float64](datapoint, "data", rowIndex, "last"),
		Bid:       datura.Peek[float64](datapoint, "data", rowIndex, "bid"),
		Ask:       datura.Peek[float64](datapoint, "data", rowIndex, "ask"),
		BidQty:    datura.Peek[float64](datapoint, "data", rowIndex, "bid_qty"),
		AskQty:    datura.Peek[float64](datapoint, "data", rowIndex, "ask_qty"),
		Change:    datura.Peek[float64](datapoint, "data", rowIndex, "change"),
		ChangePct: datura.Peek[float64](datapoint, "data", rowIndex, "change_pct"),
		High:      datura.Peek[float64](datapoint, "data", rowIndex, "high"),
		Low:       datura.Peek[float64](datapoint, "data", rowIndex, "low"),
		Volume:    datura.Peek[float64](datapoint, "data", rowIndex, "volume"),
		Timestamp: eventTime(datapoint, rowIndex),
	}
}

func bookUpdate(datapoint *datura.Artifact, rowIndex int, symbol string, eventAt time.Time) BookUpdate {
	updateType := datura.Peek[string](datapoint, "data", rowIndex, "type")

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
		price := datura.Peek[float64](datapoint, "data", rowIndex, side, levelIndex, "price")

		if price <= 0 {
			return levels
		}

		levels = append(levels, BookLevel{
			Price: price,
			Qty:   datura.Peek[float64](datapoint, "data", rowIndex, side, levelIndex, "qty"),
		})
	}
}

func tradeUpdate(datapoint *datura.Artifact, rowIndex int, symbol string, eventAt time.Time) TradeUpdate {
	return TradeUpdate{
		Symbol:    symbol,
		Side:      datura.Peek[string](datapoint, "data", rowIndex, "side"),
		Price:     datura.Peek[float64](datapoint, "data", rowIndex, "price"),
		Qty:       datura.Peek[float64](datapoint, "data", rowIndex, "qty"),
		Timestamp: eventAt,
	}
}
