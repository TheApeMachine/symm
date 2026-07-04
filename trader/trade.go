package trader

import (
	"time"

	"github.com/theapemachine/symm/kraken"
)

type Trade struct {
	history *History[kraken.TradeData]
}

func NewTrade() *Trade {
	return &Trade{
		history: NewHistory[kraken.TradeData](),
	}
}

func (trade *Trade) Measure(message kraken.TradeDataSlice) (time.Time, error) {
	var latest time.Time

	for _, msg := range message {
		if err := trade.history.Measure(msg.Symbol, msg.Timestamp, msg); err != nil {
			return time.Time{}, err
		}

		if msg.Timestamp.After(latest) {
			latest = msg.Timestamp
		}
	}

	return latest, nil
}
