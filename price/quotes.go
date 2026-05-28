package price

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/market"
)

func (prediction *Prediction) RunningMeanError() float64 {
	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	if prediction.errorCount == 0 {
		return 0
	}

	return prediction.errorSum / float64(prediction.errorCount)
}

func (prediction *Prediction) LastPrice(symbol string) float64 {
	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	if price, ok := prediction.prices[symbol]; ok && price > 0 {
		return price
	}

	return prediction.quotes[symbol].last
}

/*
LastQuote returns the cached last/bid/ask for one symbol together with the
exchange event timestamp the quote was observed at. ok is false when no
ticker has been received for that symbol yet.
*/
func (prediction *Prediction) LastQuote(symbol string) (last, bid, ask float64, at time.Time, ok bool) {
	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	quote, ok := prediction.quotes[symbol]

	if !ok {
		return 0, 0, 0, time.Time{}, false
	}

	return quote.last, quote.bid, quote.ask, quote.at, true
}

// RecentVolatility returns the EMA of per-tick absolute relative price moves
// for one symbol. Zero when the symbol has no tick history yet.
func (prediction *Prediction) RecentVolatility(symbol string) float64 {
	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	return prediction.marketMove(symbol).Value()
}

func (prediction *Prediction) observeTicker(row market.TickerRow) time.Time {
	if row.Symbol == "" || row.Last <= 0 {
		return time.Time{}
	}

	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	previous := prediction.prices[row.Symbol]

	if previous > 0 {
		relativeMove := math.Abs((row.Last - previous) / previous)

		if relativeMove > 0 {
			if _, err := prediction.marketMove(row.Symbol).Push(relativeMove); err != nil {
				errnie.Error(err)
			}
		}
	}

	eventTime := ParseEventTime(row.Timestamp)
	prediction.prices[row.Symbol] = row.Last
	prediction.quotes[row.Symbol] = lastQuote{
		last:  row.Last,
		bid:   row.Bid,
		ask:   row.Ask,
		at:    eventTime,
		local: time.Now(),
	}

	return eventTime
}

/*
ParseEventTime decodes Kraken's RFC3339Nano timestamp; returns zero time when
the string is empty or malformed (callers fall back to wall clock).
*/
func ParseEventTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}

	return time.Time{}
}
