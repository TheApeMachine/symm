package broker

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/settings"
)

/*
MeasurementBookEnricher returns a callback that attaches cached L2 depth to
measurements before they are written to the optimizer capture tape.
*/
func MeasurementBookEnricher(
	ctx context.Context,
	pool *qpool.Q,
) func(perspectives.Measurement) perspectives.Measurement {
	quotes := EnsureQuoteCache(ctx, pool)
	depth, err := settings.RequiredBookDepthLevels()

	if err != nil {
		errnie.Error(
			err,
			"broker: settings.RequiredBookDepthLevels failed, returning no-op measurement enricher",
		)

		return func(measurement perspectives.Measurement) perspectives.Measurement {
			return measurement
		}
	}

	return func(measurement perspectives.Measurement) perspectives.Measurement {
		if measurement.Symbol == "" {
			return measurement
		}

		quote, ok := quotes.Snapshot(measurement.Symbol)

		if !ok {
			return measurement
		}

		return perspectives.AttachBook(
			measurement,
			quote.Bid,
			quote.Ask,
			measurement.Last,
			quote.Book,
			depth,
		)
	}
}
