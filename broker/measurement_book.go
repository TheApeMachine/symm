package broker

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/market/settings"
)

/*
MeasurementBookEnricher returns a callback that attaches cached L2 depth to
measurements before they are written to the optimizer capture tape.
*/
func MeasurementBookEnricher(
	quotes *QuoteCache,
) func(types.Measurement) types.Measurement {
	depth, err := settings.RequiredBookDepthLevels()

	if err != nil {
		errnie.Error(
			err,
			"broker: settings.RequiredBookDepthLevels failed, returning no-op measurement enricher",
		)

		return func(measurement types.Measurement) types.Measurement {
			return measurement
		}
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
	}
}
