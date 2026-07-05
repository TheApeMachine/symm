package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type OHLC struct {
	signals []types.Signal[kraken.OHLCData]
}

func NewOHLC(signals []types.Signal[kraken.OHLCData]) *OHLC {
	return &OHLC{
		signals: signals,
	}
}

func (ohlc *OHLC) Measure(message kraken.OHLCDataSlice) ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for _, msg := range message {
		for _, signal := range ohlc.signals {
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
