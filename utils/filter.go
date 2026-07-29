package utils

import "github.com/theapemachine/symm/types"

func Measurements(thesis *types.Thesis, source types.SourceType) []*types.Measurement {
	found, ok := thesis.Measurements.Load(source)

	if !ok || found == nil {
		return nil
	}

	measurements, ok := found.([]*types.Measurement)

	if !ok || len(measurements) == 0 {
		return nil
	}

	return measurements
}

func ForSymbol(measurements []*types.Measurement, symbol string) []*types.Measurement {
	if len(measurements) == 0 {
		return nil
	}

	batch := make([]*types.Measurement, 0, len(measurements))

	for _, measurement := range measurements {
		if measurement != nil && measurement.Symbol == symbol {
			batch = append(batch, measurement)
		}
	}

	if len(batch) == 0 {
		return nil
	}

	return batch
}
