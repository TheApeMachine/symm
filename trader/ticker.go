package trader

import (
	"time"

	"github.com/theapemachine/symm/kraken"
)

type Ticker struct {
	history *History[kraken.TickerData]
}

func NewTicker() *Ticker {
	return &Ticker{
		history: NewHistory[kraken.TickerData](),
	}
}

func (ticker *Ticker) Measure(message kraken.TickerDataSlice) (time.Time, error) {
	var latest time.Time

	for _, msg := range message {
		if err := ticker.history.Measure(msg.Symbol, msg.Timestamp, msg); err != nil {
			return time.Time{}, err
		}

		if msg.Timestamp.After(latest) {
			latest = msg.Timestamp
		}
	}

	return latest, nil
}
