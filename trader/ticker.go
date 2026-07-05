package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Ticker struct {
	signals []types.Signal[kraken.TickerData]
}

func NewTicker(signals []types.Signal[kraken.TickerData]) *Ticker {
	return &Ticker{
		signals: signals,
	}
}

func (ticker *Ticker) Measure(message kraken.TickerDataSlice) ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for _, msg := range message {
		for _, signal := range ticker.signals {
			measurement, err := signal.Measure(msg, &types.CrossSection{})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				))
			}

			measurements = append(measurements, measurement...)
		}
	}

	return measurements, nil
}
