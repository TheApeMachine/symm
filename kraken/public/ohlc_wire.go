package public

import (
	"fmt"
	"time"
)

/*
IntervalBeginSec parses a Kraken interval_begin timestamp to unix seconds.
*/
func IntervalBeginSec(intervalBegin string) (int64, error) {
	if intervalBegin == "" {
		return 0, fmt.Errorf("interval_begin: empty")
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, intervalBegin)

		if err == nil {
			return parsed.Unix(), nil
		}
	}

	return 0, fmt.Errorf("interval_begin %q: unparseable", intervalBegin)
}

/*
EnrichOhlcWire adds sec (unix seconds) to a Kraken ohlc candle map for UI fanout.
*/
func EnrichOhlcWire(candle map[string]any) error {
	intervalBegin, ok := candle["interval_begin"].(string)

	if !ok {
		return fmt.Errorf("interval_begin: missing")
	}

	sec, err := IntervalBeginSec(intervalBegin)

	if err != nil {
		return err
	}

	candle["sec"] = sec

	return nil
}
