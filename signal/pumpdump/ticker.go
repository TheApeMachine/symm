package pumpdump

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

/*
tickerTrack holds the previous ticker row for volume-delta and price-direction inference.
*/
type tickerTrack struct {
	lastVolume float64
	lastPrice  float64
	ready      bool
}

func (track *tickerTrack) fold(
	state *pumpState,
	ticker market.TickerUpdate,
	at time.Time,
) (pumpReading, error) {
	if ticker.Last <= 0 {
		return pumpReading{}, errBaselineUnobserved
	}

	if !track.ready {
		track.lastVolume = ticker.Volume
		track.lastPrice = ticker.Last
		track.ready = true

		return pumpReading{}, errBaselineUnobserved
	}

	volDelta := ticker.Volume - track.lastVolume

	if volDelta <= 0 || ticker.Last == track.lastPrice {
		track.lastPrice = ticker.Last
		track.lastVolume = ticker.Volume

		return pumpReading{}, errLiftUnobserved
	}

	side := "buy"

	if ticker.Last < track.lastPrice {
		side = "sell"
	}

	track.lastPrice = ticker.Last
	track.lastVolume = ticker.Volume

	return state.fold(market.TradeUpdate{
		Symbol:    ticker.Symbol,
		Side:      side,
		Price:     ticker.Last,
		Qty:       volDelta,
		Timestamp: at,
	})
}

func tickerTimestamp(row market.TickerUpdate) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if at, err := time.Parse(layout, row.Timestamp); err == nil {
			return at, nil
		}
	}

	return time.Time{}, fmt.Errorf("ticker timestamp is required for %s", row.Symbol)
}
