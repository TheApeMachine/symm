package perspectives

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
