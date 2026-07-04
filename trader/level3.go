package trader

import (
	"time"

	"github.com/theapemachine/symm/kraken"
)

type Level3 struct {
	history *History[kraken.Level3Data]
}

func NewLevel3() *Level3 {
	return &Level3{
		history: NewHistory[kraken.Level3Data](),
	}
}

func (level3 *Level3) Measure(message kraken.Level3DataSlice) (time.Time, error) {
	var latest time.Time

	for _, msg := range message {
		if err := level3.history.Measure(msg.Symbol, msg.Timestamp, msg); err != nil {
			return time.Time{}, err
		}

		if msg.Timestamp.After(latest) {
			latest = msg.Timestamp
		}
	}

	return latest, nil
}
