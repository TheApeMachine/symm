package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Trade struct {
	signals []types.Signal[kraken.TradeData]
}

func NewTrade(signals []types.Signal[kraken.TradeData]) *Trade {
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

			measurements = append(measurements, measurement...)
		}
	}

	return measurements, nil
}
