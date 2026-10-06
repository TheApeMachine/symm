package data

/*
priorMetric holds the transferable Welford state for one metric label.
*/
type priorMetric struct {
	center float64
	scale  float64
}

/*
priorSnapshot holds Measurement statistics that must survive arena Free so
the next Measurement from the same ArenaOwner can continue the regime.
Heap-allocated; never stored inside an arena generation.
*/
type priorSnapshot struct {
	samples    int64
	prediction float64
	metrics    map[string]priorMetric
}

func (snapshot *priorSnapshot) metric(label string) (priorMetric, bool) {
	if snapshot == nil || snapshot.metrics == nil {
		return priorMetric{}, false
	}

	seed, ok := snapshot.metrics[label]
	return seed, ok
}

func snapshotFrom(measurement *Measurement) *priorSnapshot {
	if measurement == nil {
		return nil
	}

	metrics := make(map[string]priorMetric, len(measurement.metrics))

	for _, entry := range measurement.metrics {
		if entry == nil || entry.Metric == nil {
			continue
		}

		metrics[entry.Metric.Label] = priorMetric{
			center: entry.Metric.center,
			scale:  entry.Metric.scale,
		}
	}

	return &priorSnapshot{
		samples:    measurement.samples,
		prediction: measurement.prediction,
		metrics:    metrics,
	}
}
