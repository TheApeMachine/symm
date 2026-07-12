package trader

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
TradeBatch measures one ordered event batch in event-major order. A signal
failure stops the batch at the exact event boundary where it occurred.
*/
type TradeBatch struct {
	signals  []types.Signal[any]
	events   []types.Event
	snapshot *types.CrossSection
}

func NewTradeBatch(
	signals []types.Signal[any],
	events []types.Event,
	snapshot *types.CrossSection,
) *TradeBatch {
	return &TradeBatch{
		signals:  signals,
		events:   events,
		snapshot: snapshot,
	}
}

func (batch *TradeBatch) Measure() ([]*types.Measurement, error) {
	if len(batch.events) == 0 || len(batch.signals) == 0 {
		return nil, nil
	}

	measurements := make([]*types.Measurement, 0)

	for _, event := range batch.events {
		row, ok := event.Row.(kraken.TradeData)

		if !ok {
			continue
		}

		for _, signal := range batch.signals {
			result, err := signal.Measure(row, batch.snapshot)

			if err != nil {
				return nil, err
			}

			batch.stamp(result, event.Price)
			measurements = append(measurements, result...)
		}
	}

	return measurements, nil
}

func (batch *TradeBatch) stamp(measurements []*types.Measurement, price float64) {
	if price <= 0 {
		return
	}

	for _, measurement := range measurements {
		if measurement.Metrics == nil {
			measurement.Metrics = map[string]float64{}
		}

		measurement.Metrics["price"] = price
	}
}
