package utils

import "github.com/theapemachine/symm/types"

func Measurements(thesis *types.Thesis, source types.SourceType) []*types.Measurement {
	found, ok := thesis.Measurements.Load(types.SourceHawkes)

	if !ok || found == nil {
		return nil
	}

	measurements, ok := found.([]*types.Measurement)

	if !ok || len(measurements) == 0 {
		return nil
	}

	return measurements
}
