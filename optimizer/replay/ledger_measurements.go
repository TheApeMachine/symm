package replay

import (
	"sort"

	"github.com/theapemachine/symm/market/perspectives"
)

type replayMeasurements struct {
	global  map[perspectives.SourceType]perspectives.Measurement
	symbols map[string]map[perspectives.SourceType]perspectives.Measurement
}

func newReplayMeasurements() *replayMeasurements {
	return &replayMeasurements{
		global:  make(map[perspectives.SourceType]perspectives.Measurement),
		symbols: make(map[string]map[perspectives.SourceType]perspectives.Measurement),
	}
}

func (measurements *replayMeasurements) Add(
	measurement perspectives.Measurement,
) {
	if measurement.Symbol == "" {
		measurements.global[measurement.Source] = measurement

		return
	}

	symbolRows, ok := measurements.symbols[measurement.Symbol]

	if !ok {
		symbolRows = make(map[perspectives.SourceType]perspectives.Measurement)
		measurements.symbols[measurement.Symbol] = symbolRows
	}

	symbolRows[measurement.Source] = measurement
}

func (measurements *replayMeasurements) Snapshot(
	symbol string,
) []perspectives.Measurement {
	rows := make(
		[]perspectives.Measurement, 0,
		len(measurements.global)+len(measurements.symbols[symbol]),
	)

	rows = append(rows, sortedMeasurementsBySource(measurements.global)...)
	rows = append(rows, sortedMeasurementsBySource(measurements.symbols[symbol])...)

	return rows
}

func sortedMeasurementsBySource(
	rows map[perspectives.SourceType]perspectives.Measurement,
) []perspectives.Measurement {
	sorted := make([]perspectives.Measurement, 0, len(rows))

	for _, measurement := range rows {
		sorted = append(sorted, measurement)
	}

	sort.Slice(sorted, func(leftIndex, rightIndex int) bool {
		return sorted[leftIndex].Source < sorted[rightIndex].Source
	})

	return sorted
}
