package data

/*
Lift flattens a set of measurements into one observation keyed by
source-qualified metric name, resolving metric values through their Readout
so no naked unconditioned numbers escape into the system.

A metric's identity is its source and its own label — "hawkes/arrival_rate" —
so measurements from different signals never collide and a consumer names the
evidence it wants without knowing which measurement carried it.

A measurement carrying an Err is skipped rather than discarded: one failed
signal must not erase every other signal's metrics from the same observation.
The error is returned alongside so a caller can report it, but the metrics
that were successfully measured still reach the observation.
*/
func Lift(measurements []*Measurement[float64]) (map[string]float64, error) {
	observation := make(map[string]float64)

	var failure error

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		if measurement.Err != nil {
			if failure == nil {
				failure = measurement.Err
			}

			continue
		}

		for label, metric := range measurement.Metrics {
			readout := measurement.Readout(label)

			if readout != nil {
				observation[measurement.Source+"/"+label] = readout.Value()
				continue
			}

			observation[measurement.Source+"/"+label] = metric.Raw
		}
	}

	return observation, failure
}

/*
LiftReadouts flattens a set of measurements into Readouts keyed by
source-qualified metric name, preserving the full quality context (maturity,
SNR, credibility, and corroborations) for downstream logic layers.
*/
func LiftReadouts(measurements []*Measurement[float64]) (map[string]*Readout, error) {
	readouts := make(map[string]*Readout)

	var failure error

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		if measurement.Err != nil {
			if failure == nil {
				failure = measurement.Err
			}

			continue
		}

		for label := range measurement.Metrics {
			readout := measurement.Readout(label)

			if readout != nil {
				readouts[measurement.Source+"/"+label] = readout
			}
		}
	}

	return readouts, failure
}

