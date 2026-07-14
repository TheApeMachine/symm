package tests

import "github.com/theapemachine/symm/types"

/*
MeasurementFields maps each metric to its latest raw value for one symbol.
*/
func MeasurementFields(
	measurements []*types.Measurement,
	symbol string,
) map[types.MetricType]float64 {
	fields := map[types.MetricType]float64{}

	for _, measurement := range measurements {
		if measurement.Symbol == symbol {
			fields[measurement.Metric] = measurement.Raw
		}
	}

	return fields
}
