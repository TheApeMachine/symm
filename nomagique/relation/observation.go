package relation

import (
	"time"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Observation is one stored measurement fact for one Coordinate. It preserves
the raw signed value, the observation window, quality provenance, and the
originating Measurement ID.

The Measurement SNR is provenance only. It is never the default Relation
variable; Relation uses the actual signed metric value or its causal
standardized residual.
*/
type Observation struct {
	// Coordinate is the typed coordinate identity this observation belongs to.
	Coordinate Coordinate
	// Raw is the signed metric value.
	Raw float64
	// From is the start of the observation window, when known.
	From time.Time
	// At is the as-of / emit instant.
	At time.Time
	// Maturity is the measurement-level maturity provenance.
	Maturity float64
	// SNR is the measurement-level SNR provenance when defined. Undefined is
	// carried by SNRDefined=false; a genuine measured zero departure is
	// SNR=0 with SNRDefined=true. The two are different facts.
	SNR float64
	// SNRDefined reports whether the SNR is a measured value rather than the
	// absence of an estimable noise model.
	SNRDefined bool
	// MeasurementID is the originating measurement identifier.
	MeasurementID string
}

/*
AppendMeasurement splits one shared types.Measurement into per-coordinate
Observations. Every valid metric becomes an independent observational fact;
nothing is collapsed into a signal-level scalar. A measurement carrying an
error is rejected as a whole.
*/
func AppendMeasurement(
	measurement *nmtypes.Measurement,
	epoch uint64,
) []Observation {
	if measurement == nil || measurement.Err != nil {
		return nil
	}

	observations := make([]Observation, 0, len(measurement.Metrics))

	for label, metric := range measurement.Metrics {
		if metric == nil {
			continue
		}

		metricName, side := ParseMetricSide(label)
		observations = append(observations, Observation{
			Coordinate: Coordinate{
				Symbol:    measurement.Symbol,
				Peer:      measurement.Peer,
				Source:    measurement.Source,
				Metric:    metricName,
				Side:      side,
				Unit:      metric.Unit,
				Timescale: metric.Timescale,
				Epoch:     epoch,
			},
			Raw:           metric.Raw,
			From:          measurement.ObservedFrom,
			At:            measurement.At,
			Maturity:      measurement.Maturity,
			SNR:           measurement.SNR,
			SNRDefined:    measurement.SNRDefined,
			MeasurementID: measurement.ID,
		})
	}

	return observations
}
