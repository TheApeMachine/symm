package broker

import (
	"fmt"

	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/market/settings"
)

/*
MeasurementBookEnricher returns a callback that attaches cached L2 depth to
measurements before they are written to the optimizer capture tape.
*/
func MeasurementBookEnricher(
	quotes *QuoteCache,
) (func(types.Measurement) types.Measurement, error) {
	if quotes == nil {
		return nil, fmt.Errorf("broker: quote cache is required")
	}

	depth, err := settings.RequiredBookDepthLevels()

	if err != nil {
		return nil, fmt.Errorf("broker: book depth: %w", err)
	}

	return func(measurement types.Measurement) types.Measurement {
		if measurement.Symbol == "" {
			return measurement
		}

		quote, ok := quotes.Snapshot(measurement.Symbol)

		if !ok {
			return measurement
		}

		return types.AttachBook(
			measurement,
			quote.Bid,
			quote.Ask,
			measurement.Last,
			quote.Book,
			depth,
		)
	}, nil
}
