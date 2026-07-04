package trader

import (
	"time"

	"github.com/theapemachine/symm/kraken"
)

type OHLC struct {
	history *History[kraken.OHLCData]
}

func NewOHLC() *OHLC {
	return &OHLC{
		history: NewHistory[kraken.OHLCData](),
	}
}

func (ohlc *OHLC) Measure(message kraken.OHLCDataSlice) (time.Time, error) {
	var latest time.Time

	for _, msg := range message {
		if err := ohlc.history.Measure(msg.Symbol, msg.Timestamp, msg); err != nil {
			return time.Time{}, err
		}

		if msg.Timestamp.After(latest) {
			latest = msg.Timestamp
		}
	}

	return latest, nil
}
