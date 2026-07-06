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
		for _, signal := range level3.signals {
			measurement, err := signal.Measure(msg, &types.CrossSection{})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				))
			}

			price := 0.0
			if len(msg.Bids) > 0 && len(msg.Asks) > 0 {
				price = (msg.Bids[0].LimitPrice + msg.Asks[0].LimitPrice) / 2
			}

			for _, item := range measurement {
				if item.Metrics == nil {
					item.Metrics = map[string]float64{}
				}

				if price > 0 {
					item.Metrics["price"] = price
				}
			}

			measurements = append(measurements, measurement...)
		}
	}

	return measurements, nil
}
