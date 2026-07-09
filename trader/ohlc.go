package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type OHLC struct {
	signals []types.Signal[any]
}

func NewOHLC(signals []types.Signal[any]) *OHLC {
	return &OHLC{
		signals: signals,
	}
}

func (ohlc *OHLC) Measure(message kraken.OHLCDataSlice) ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for _, msg := range message {
		results := measureSignals(ohlc.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
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

			measurements = append(measurements, result.measurements...)
		}
	}

	return measurements, nil
}
