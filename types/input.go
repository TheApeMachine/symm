package types

import (
	"time"

	"github.com/theapemachine/symm/kraken"
)

type Input struct {
	Role   string
	At     time.Time
	Ticker []kraken.TickerData
	Trade  []kraken.TradeData
	OHLC   kraken.OHLCDataSlice
	Book   kraken.BookDataSlice
	Level3 kraken.Level3DataSlice
}

func (input Input) Latest() time.Time {
	latest := input.At

	for _, row := range input.Ticker {
		latest = latestTime(latest, row.Timestamp)
	}

	for _, row := range input.Trade {
		latest = latestTime(latest, row.Timestamp)
	}

	for _, row := range input.OHLC {
		latest = latestTime(latest, row.Timestamp)
	}

	for _, row := range input.Book {
		latest = latestTime(latest, row.Timestamp)
	}

	for _, row := range input.Level3 {
		latest = latestTime(latest, row.Timestamp)
	}

	return latest
}

func latestTime(current time.Time, candidate time.Time) time.Time {
	if candidate.IsZero() || !candidate.After(current) {
		return current
	}

	return candidate
}
