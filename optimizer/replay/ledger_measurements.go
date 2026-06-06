package replay

import (
	"sort"

	"github.com/theapemachine/symm/market/perspectives/types"
)

type replayMeasurements struct {
	global  map[types.SourceType]types.Measurement
	symbols map[string]map[types.SourceType]types.Measurement
}

func newReplayMeasurements() *replayMeasurements {
	return &replayMeasurements{
		global:  make(map[types.SourceType]types.Measurement),
		symbols: make(map[string]map[types.SourceType]types.Measurement),
	}
}

func (measurements *replayMeasurements) Add(
	measurement types.Measurement,
) {
	if measurement.Symbol == "" {
		measurements.global[measurement.Source] = measurement

		return
	}

	symbolRows, ok := measurements.symbols[measurement.Symbol]

	if !ok {
		symbolRows = make(map[types.SourceType]types.Measurement)
		measurements.symbols[measurement.Symbol] = symbolRows
	}

	symbolRows[measurement.Source] = measurement
}

func (measurements *replayMeasurements) Snapshot(
	symbol string,
) []types.Measurement {
	rows := make(
		[]types.Measurement, 0,
		len(measurements.global)+len(measurements.symbols[symbol]),
	)

	rows = append(rows, sortedMeasurementsBySource(measurements.global)...)
	rows = append(rows, sortedMeasurementsBySource(measurements.symbols[symbol])...)

	return rows
}

func sortedMeasurementsBySource(
	rows map[types.SourceType]types.Measurement,
) []types.Measurement {
	sorted := make([]types.Measurement, 0, len(rows))

	for _, measurement := range rows {
		sorted = append(sorted, measurement)
	}

	sort.Slice(sorted, func(leftIndex, rightIndex int) bool {
		return sorted[leftIndex].Source < sorted[rightIndex].Source
	})

	return sorted
}
