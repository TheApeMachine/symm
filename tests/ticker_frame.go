package tests

import (
	"fmt"
	"time"
)

/*
TickerUpdate builds one Kraken ticker update frame for a symbol and last price.
*/
func TickerUpdate(symbol string, last float64) []byte {
	frame := map[string]any{
		"channel": "ticker",
		"type":    "update",
		"data": []map[string]any{{
			"symbol":    symbol,
			"last":      fmt.Sprintf("%g", last),
			"timestamp": time.Unix(1, 0).UTC().Format(time.RFC3339Nano),
		}},
	}

	return marshalFrame(frame)
}
