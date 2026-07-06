package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Ticker struct {
	signals      []types.Signal[any]
	crossSection *types.CrossSection
}

func NewTicker(
	signals []types.Signal[any],
	crossSection *types.CrossSection,
) *Ticker {
	return &Ticker{
		signals:      signals,
		crossSection: crossSection,
	}
}

func (ticker *Ticker) Measure(message kraken.TickerDataSlice) ([]*types.Measurement, error) {
	if ticker.crossSection != nil {
		if err := ticker.crossSection.Observe(message); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			))
		}
	}

	measurements := make([]*types.Measurement, 0)

	for _, msg := range message {
		for _, signal := range ticker.signals {
			measurement, err := signal.Measure(msg, ticker.crossSection)

			if err != nil {
				return nil, errnie.Error(errnie.Err(
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
