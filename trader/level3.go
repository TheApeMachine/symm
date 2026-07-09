package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Level3 struct {
	signals []types.Signal[any]
}

func NewLevel3(signals []types.Signal[any]) *Level3 {
	return &Level3{
		signals: signals,
	}
}

func (level3 *Level3) Measure(message kraken.Level3DataSlice) ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for _, msg := range message {
		results := measureSignals(level3.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
			return signal.Measure(msg, &types.CrossSection{})
		})

		for _, result := range results {
			if result.err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					result.err.Error(),
					result.err,
				))
				continue
			}

			price := 0.0
			if len(msg.Bids) > 0 && len(msg.Asks) > 0 {
				price = (msg.Bids[0].LimitPrice + msg.Asks[0].LimitPrice) / 2
			}

			for _, item := range result.measurements {
				if item.Metrics == nil {
					item.Metrics = map[string]float64{}
				}

				if price > 0 {
					item.Metrics["price"] = price
				}
			}

			measurements = append(measurements, result.measurements...)
		}
	}

	return measurements, nil
}
