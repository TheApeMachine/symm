package types

/*
ResonanceMeasurement holds the conditioned readings and mark extracted at
measurement ingestion time.
*/
type ResonanceMeasurement struct {
	Tick     int64
	Mark     float64
	Readings map[string]float64 // "source:symbol:metric_key" -> normalized value
}

/*
MeasurementToResonance presents predictive coding with the task-relevant state,
not every normalized number a signal happened to publish.

The direction head is asked whether the next distinct mark is up or down. It
therefore receives competing hypothesis channels, their separation, and the
small set of signed or authenticity-bearing summaries that can change that
answer. Prices, quantities, counts, fitted kernel parameters, and duplicated
bookkeeping summaries remain available to the rest of logic but do not expand
the coder's schema or teach it transport activity.
*/
func MeasurementToResonance(
	symbolName string,
	measurement *Measurement,
) *ResonanceMeasurement {
	if measurement == nil {
		return nil
	}

	mark := 0.0

	if bid, hasBid := measurement.Metadata["bid"]; hasBid {
		if ask, hasAsk := measurement.Metadata["ask"]; hasAsk && bid > 0 && ask > 0 {
			mark = (bid + ask) / 2
		}
	}

	if mark == 0 {
		if value, found := measurement.Metadata["last_price"]; found && value > 0 {
			mark = value
		}
	}

	if mark == 0 {
		if value, found := measurement.Metadata["trade_price"]; found && value > 0 {
			mark = value
		}
	}

	var readings map[string]float64

	if len(measurement.Metrics) > 0 {
		readings = make(map[string]float64)

		for key, sample := range measurement.Metrics {
			if sample.Normalized == nil || !ResonanceMetricAllowed(measurement.Source, key) {
				continue
			}

			identity := string(measurement.Source) + ":" + symbolName + ":" + key
			readings[identity] = *sample.Normalized
		}
	}

	if len(readings) == 0 && mark == 0 {
		return nil
	}

	return &ResonanceMeasurement{
		Tick:     measurement.Tick,
		Mark:     mark,
		Readings: readings,
	}
}

/*
ResonanceMetricAllowed declares the stable, semantically relevant feature
surface of the direction task. SignalMetricGroups remains the authority for
which metrics are competing hypotheses; the switch adds signed flow, temporal
direction, criticality, decay urgency, and book-honesty summaries that are not
category competitors but still carry directional information.
*/
func ResonanceMetricAllowed(source SourceType, key string) bool {
	metric, _ := ParseMetricKey(key)

	if metric == MetricHypothesisSeparation {
		return true
	}

	if groups, found := SignalMetricGroups[source]; found {
		if membership, exists := groups[key]; exists && membership.Competes {
			return true
		}
	}

	switch metric {
	case MetricNetFraction,
		MetricSigned,
		MetricSignedCorrelation,
		MetricSignedContempCorrelation,
		MetricSignedLagCorrelation,
		MetricSignedLagDirection,
		MetricLagFraction,
		MetricSpectralRadius,
		MetricUrgency,
		MetricBluffScore,
		MetricVacuumScore,
		MetricSupportScore:
		return true
	default:
		return false
	}
}
