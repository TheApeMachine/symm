package perspectives

/*
GaugeFactor is one sub-metric that drives a signal's dashboard confidence reading.
Gauges show the fused Strength/SNR; factors expose the underlying components for
operator debugging without flattening the primary dial.
*/
type GaugeFactor struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

/*
GaugeFactorsFrom extracts named fields from a wire row into gauge factors.
*/
func GaugeFactorsFrom(row map[string]any, names ...string) []GaugeFactor {
	if len(row) == 0 || len(names) == 0 {
		return nil
	}

	factors := make([]GaugeFactor, 0, len(names))

	for _, name := range names {
		value, ok := row[name].(float64)

		if !ok || value == 0 {
			continue
		}

		factors = append(factors, GaugeFactor{Name: name, Value: value})
	}

	return factors
}

/*
WithGaugeFactors returns measurement with factors attached for dashboard telemetry.
*/
func WithGaugeFactors(measurement Measurement, factors []GaugeFactor) Measurement {
	if len(factors) > 0 {
		measurement.Factors = factors
	}

	return measurement
}
