package types

import "time"

/*
ObservationMeasurement builds one valid observation-layer measurement with the
shared temporal and validity contract used across migrated signals.
*/
func ObservationMeasurement(
	source SourceType,
	stream StreamType,
	metric MetricType,
	subject SubjectType,
	symbol string,
	at time.Time,
	unit MeasurementUnit,
	raw float64,
	maturity float64,
) *Measurement {
	return observationMeasurement(
		source, stream, metric, subject, symbol, SideNone, at, unit, raw, maturity,
		normalizedObservation(unit, raw, nil),
	)
}

/*
ObservationNormalizedMeasurement retains an explicit normalized value alongside
the raw observation when the signal already derived a baseline-relative score.
*/
func ObservationNormalizedMeasurement(
	source SourceType,
	stream StreamType,
	metric MetricType,
	subject SubjectType,
	symbol string,
	at time.Time,
	unit MeasurementUnit,
	raw float64,
	maturity float64,
	normalized *float64,
) *Measurement {
	return observationMeasurement(
		source, stream, metric, subject, symbol, SideNone, at, unit, raw, maturity,
		normalized,
	)
}

/*
ObservationSideMeasurement preserves directional semantics for touch and flow
evidence emitted by migrated signals.
*/
func ObservationSideMeasurement(
	source SourceType,
	stream StreamType,
	metric MetricType,
	subject SubjectType,
	symbol string,
	side MeasurementSide,
	at time.Time,
	unit MeasurementUnit,
	raw float64,
	maturity float64,
) *Measurement {
	return observationMeasurement(
		source, stream, metric, subject, symbol, side, at, unit, raw, maturity,
		normalizedObservation(unit, raw, nil),
	)
}

/*
ObservationSideNormalizedMeasurement preserves directional semantics and an
explicit normalized value for touch and flow evidence.
*/
func ObservationSideNormalizedMeasurement(
	source SourceType,
	stream StreamType,
	metric MetricType,
	subject SubjectType,
	symbol string,
	side MeasurementSide,
	at time.Time,
	unit MeasurementUnit,
	raw float64,
	maturity float64,
	normalized *float64,
) *Measurement {
	return observationMeasurement(
		source, stream, metric, subject, symbol, side, at, unit, raw, maturity,
		normalized,
	)
}

func observationMeasurement(
	source SourceType,
	stream StreamType,
	metric MetricType,
	subject SubjectType,
	symbol string,
	side MeasurementSide,
	at time.Time,
	unit MeasurementUnit,
	raw float64,
	maturity float64,
	normalized *float64,
) *Measurement {
	return &Measurement{
		Source:     source,
		Stream:     stream,
		Metric:     metric,
		Subject:    subject,
		Symbol:     symbol,
		Side:       side,
		At:         at,
		Unit:       unit,
		Raw:        raw,
		Normalized: normalized,
		Maturity:   maturity,
		Validity: MeasurementValidity{
			State:     ValidityValid,
			Readiness: ReadinessObservation,
		},
		Scale: ScaleReference{
			Kind:    ScaleObservationWindow,
			From:    at,
			Through: at,
		},
	}
}

func normalizedObservation(
	unit MeasurementUnit,
	raw float64,
	normalized *float64,
) *float64 {
	if normalized != nil {
		return normalized
	}

	if unit == UnitDimensionless {
		return NormalizeFinite(raw)
	}

	return nil
}
