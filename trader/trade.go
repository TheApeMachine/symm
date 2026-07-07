package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Trade struct {
	signals []types.Signal[any]
}

func NewTrade(signals []types.Signal[any]) *Trade {
	return &Trade{
		signals: signals,
	}
}

func (trade *Trade) Measure(message kraken.TradeDataSlice) ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for _, msg := range message {
		for _, signal := range trade.signals {
			measurement, err := signal.Measure(msg, &types.CrossSection{})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				))
			}

			for _, item := range measurement {
				if item.Metrics == nil {
					item.Metrics = map[string]float64{}
				}

				if msg.Price.Float64() > 0 {
					item.Metrics["price"] = msg.Price.Float64()
				}
			}

			measurements = append(measurements, measurement...)
		}
	}

	return measurements, nil
}
