package trader

import (
	"fmt"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
observe dispatches one ordered raw frame to its typed stream observer.
*/
func (level3 *Level3) observe(frame level3Frame) {
	switch frame.stream {
	case channelTrade:
		level3.observeTrades(frame.raw)
	case channelLevel3:
		level3.observeLevel3(frame.raw)
	default:
		level3.fail(fmt.Errorf("trader: unsupported microstructure stream %s", frame.stream))
	}
}

func (level3 *Level3) observeTrades(raw []byte) {
	measurements := make([]*types.Measurement, 0)

	for _, row := range kraken.NewTrade(raw).Data {
		measurements = append(measurements, level3.measure(row, row.Price.Float64())...)
	}

	level3.publish(measurements)
}

func (level3 *Level3) observeLevel3(raw []byte) {
	rows := kraken.NewLevel3(raw).Data

	if len(rows) == 0 {
		return
	}

	measurements := make([]*types.Measurement, 0, len(rows))

	for _, row := range rows {
		if level3.recoveringDelta(row) {
			continue
		}

		result := level3.observeRow(row)

		if !result.AdvanceReady {
			level3.recover(row.Symbol, result.State.InvalidReason)
			continue
		}

		level3.recovered(row)
		level3.status.Store(types.READY)
		measurements = append(measurements, level3.measure(row, level3MidPrice(row))...)
	}

	level3.publish(measurements)
}

func (level3 *Level3) observeRow(row kraken.Level3Data) manifold.ProcessResult {
	pair, err := level3.instrument.Pair(row.Symbol)

	if err != nil {
		level3.scheduler.Remove(row.Symbol)
		level3.fail(err)
		return manifold.ProcessResult{}
	}

	result := level3.analyzer.ObserveLevel3(
		row,
		pair.PricePrecision,
		pair.QtyPrecision,
		level3.book,
	)

	if result.AdvanceReady {
		level3.scheduler.Mark(row.Symbol)
		return result
	}

	level3.scheduler.Remove(row.Symbol)
	return result
}

func (level3 *Level3) measure(input any, price float64) []*types.Measurement {
	results := measureSignals(level3.signals, func(signal types.Signal[any]) (
		[]*types.Measurement, error,
	) {
		return signal.Measure(input, &types.CrossSection{})
	})
	measurements := make([]*types.Measurement, 0, len(results))

	for _, result := range results {
		if result.err != nil {
			continue
		}

		for _, measurement := range result.measurements {
			if measurement.Metrics == nil {
				measurement.Metrics = map[string]float64{}
			}

			if price > 0 {
				measurement.Metrics["price"] = price
			}

			measurements = append(measurements, measurement)
		}
	}

	return measurements
}

func (level3 *Level3) publish(measurements []*types.Measurement) {
	for _, measurement := range measurements {
		if err := level3.mailbox.Store(measurement); err != nil {
			level3.fail(err)
			return
		}
	}

	if len(measurements) == 0 {
		return
	}

	select {
	case level3.uiHub <- datura.Map[any]{"measurements": measurements}.Marshal():
	default:
	}
}

func level3MidPrice(row kraken.Level3Data) float64 {
	if len(row.Bids) == 0 || len(row.Asks) == 0 {
		return 0
	}

	return (row.Bids[0].LimitPrice + row.Asks[0].LimitPrice) / 2
}
