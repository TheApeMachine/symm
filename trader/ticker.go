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
		results := measureSignals(ticker.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
			return signal.Measure(msg, ticker.crossSection)
		})

		for _, result := range results {
			if result.err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					result.err.Error(),
					result.err,
				))
			}

			price := msg.Last.Float64()
			if price <= 0 {
				price = (msg.Bid.Float64() + msg.Ask.Float64()) / 2
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
