package relation

import (
	"strings"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
Observation is one stored measurement fact for one Coordinate. It preserves
the raw signed value, the observation window, and the originating
Measurement ID.
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
	// MeasurementID is the originating (finalized) measurement identifier.
	MeasurementID uint32
}

/*
splitMeasurement splits one data.Measurement into per-coordinate Observations.
A Measurement cannot enumerate its metrics, so the caller names the
coordinates it projects: each coordinate's Metric (and Side, rendered as
"metric:side") is the key read from the Measurement, while Symbol, Source,
Peer (metadata "peer") and Epoch are stamped from the Measurement and the
request. Every requested metric becomes an independent observational fact;
nothing is collapsed into a signal-level scalar. A metric that cannot be read
(unfinalized, missing, or carrying the Measurement's error) rejects the
measurement as a whole.
*/
func splitMeasurement(
	measurement *data.Measurement,
	coordinates []Coordinate,
	epoch uint64,
) ([]Observation, error) {
	if measurement == nil {
		return nil, nil
	}

	observations := make([]Observation, 0, len(coordinates))
	peer := measurement.Meta("peer")

	for _, coordinate := range coordinates {
		key := coordinate.Metric

		if coordinate.Side != "" {
			key += ":" + coordinate.Side
		}

		entry := measurement.Read(key)

		if entry.Err != nil {
			return nil, entry.Err
		}

		coordinate.Symbol = measurement.Label
		coordinate.Peer = peer
		coordinate.Source = measurement.Source
		coordinate.Epoch = epoch

		observations = append(observations, Observation{
			Coordinate:    coordinate,
			Raw:           entry.Metric.Raw,
			From:          measurement.From,
			At:            measurement.At,
			MeasurementID: measurement.ID,
		})
	}

	return observations, nil
}

/*
parseMetricSide splits a projected metric label into its base metric and side
suffix. The signal boundary keys metrics as "metric" or "metric:side", so the
first colon separates the side suffix; namespaced metric names use '/' and
are never split.
*/
func parseMetricSide(label string) (metric string, side string) {
	if index := strings.IndexByte(label, ':'); index >= 0 {
		return label[:index], label[index+1:]
	}

	return label, ""
}
