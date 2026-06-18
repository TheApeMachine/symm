package toxicity

import (
	"fmt"
	"strconv"
	"time"
)

/*
Pair holds minimal instrument metadata for toxicity tracker keying.
*/
type Pair struct {
	Wsname   string
	TickSize string
}

/*
TickSizeFloat parses the configured tick increment.
*/
func (pair Pair) TickSizeFloat() (float64, error) {
	if pair.TickSize == "" {
		return 0, fmt.Errorf("toxicity: tick size unset")
	}

	value, err := strconv.ParseFloat(pair.TickSize, 64)

	if err != nil {
		return 0, fmt.Errorf("toxicity: tick size %q: %w", pair.TickSize, err)
	}

	return value, nil
}

/*
BookLevel is one price/qty level in a book update payload.
*/
type BookLevel struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

/*
BookUpdate is the decoded book frame used by the toxicity tracker.
*/
type BookUpdate struct {
	Symbol    string      `json:"symbol"`
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
}
