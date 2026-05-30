package perspectives

/*
FinalizeSNR records the raw fused strength for dashboards and applies an optional
noise-floor scorer for playbook branches. Gauges read Strength; trees read SNR.
*/
func FinalizeSNR(
	measurement Measurement,
	raw float64,
	score func(float64) float64,
) Measurement {
	measurement.Strength = raw

	if score != nil {
		measurement.SNR = score(raw)
	}

	if measurement.SNR <= 0 && raw > 0 {
		measurement.SNR = raw
	}

	return measurement
}

/*
GaugeValue returns the reading to show on the dashboard dial: live fused strength,
not the post-warmup playbook SNR.
*/
func GaugeValue(measurement Measurement) float64 {
	if measurement.Strength > 0 {
		return measurement.Strength
	}

	return measurement.SNR
}
