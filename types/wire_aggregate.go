package types

import (
	"time"

	"github.com/theapemachine/datura"
)

/*
wireKey identifies one source×symbol×at observation without string formatting.
*/
type wireKey struct {
	source SourceType
	symbol string
	atNano int64
}

/*
wireGroup is one source×symbol×at observation assembled for the compact UI wire.
*/
type wireGroup struct {
	source            SourceType
	symbol            string
	at                time.Time
	maturity          float64
	validity          MeasurementValidity
	scale             ScaleReference
	metrics           datura.Map[any]
	normalizedMetrics datura.Map[any]
}

/*
AggregateMeasurements projects Thesis Measurements onto the compact UI wire:
one frame per source×symbol×at with metrics and normalized_metrics maps.
Thesis already stores that shape; this copies candidates (latest epoch plus
fit-parameter rows) into datura frames.
*/
func AggregateMeasurements(measurements []*Measurement) []datura.Map[any] {
	candidates := wireCandidates(measurements)

	if len(candidates) == 0 {
		return nil
	}

	groups := make(map[wireKey]*wireGroup, len(candidates))
	order := make([]wireKey, 0, len(candidates))

	for _, measurement := range candidates {
		if measurement == nil ||
			measurement.Symbol == "" ||
			len(measurement.Metrics) == 0 {
			continue
		}

		key := wireGroupKey(measurement)
		group, exists := groups[key]

		if !exists {
			group = newWireGroup(measurement)
			groups[key] = group
			order = append(order, key)
		}

		group.accumulate(measurement)
	}

	out := make([]datura.Map[any], 0, len(order))

	for _, key := range order {
		out = append(out, groups[key].frame())
	}

	return out
}

func wireCandidates(measurements []*Measurement) []*Measurement {
	latest := FilterLatest(measurements)

	if len(latest) == 0 {
		return nil
	}

	latestKeys := make(map[wireKey]struct{}, len(latest))

	for _, measurement := range latest {
		if measurement == nil {
			continue
		}

		latestKeys[wireGroupKey(measurement)] = struct{}{}
	}

	out := make([]*Measurement, 0, len(latest)+8)
	out = append(out, latest...)

	for _, measurement := range measurements {
		if measurement == nil || !measurement.fitParameter() {
			continue
		}

		if _, exists := latestKeys[wireGroupKey(measurement)]; exists {
			continue
		}

		out = append(out, measurement)
	}

	return out
}

func wireGroupKey(measurement *Measurement) wireKey {
	return wireKey{
		source: measurement.Source,
		symbol: measurement.Symbol,
		atNano: measurement.At.UTC().UnixNano(),
	}
}

func newWireGroup(measurement *Measurement) *wireGroup {
	return &wireGroup{
		source:   measurement.Source,
		symbol:   measurement.Symbol,
		at:       measurement.At,
		maturity: measurement.Maturity,
		validity: measurement.Validity,
		scale:    measurement.Scale,
		metrics:  make(datura.Map[any], len(measurement.Metrics)),
	}
}

func (group *wireGroup) accumulate(measurement *Measurement) {
	if group.scale.Kind == "" &&
		group.scale.From.IsZero() &&
		group.scale.Through.IsZero() {
		group.scale = measurement.Scale
	}

	if measurement.Maturity > group.maturity {
		group.maturity = measurement.Maturity
	}

	if measurement.Validity.State != "" || measurement.Validity.Readiness != "" {
		group.validity = measurement.Validity
	}

	for key, sample := range measurement.Metrics {
		group.metrics[key] = sample.Raw

		if sample.Normalized != nil {
			if group.normalizedMetrics == nil {
				group.normalizedMetrics = make(datura.Map[any], len(measurement.Metrics))
			}

			group.normalizedMetrics[key] = *sample.Normalized
		}
	}
}

func (group *wireGroup) frame() datura.Map[any] {
	frame := datura.Map[any]{
		"source":  group.source,
		"symbol":  group.symbol,
		"at":      group.at,
		"metrics": group.metrics,
	}

	if group.maturity > 0 {
		frame["maturity"] = group.maturity
	}

	if group.validity.State != "" || group.validity.Readiness != "" {
		frame["validity"] = group.validity
	}

	if group.scale.Kind != "" ||
		!group.scale.From.IsZero() ||
		!group.scale.Through.IsZero() {
		frame["scale"] = group.scale
	}

	if len(group.normalizedMetrics) > 0 {
		frame["normalized_metrics"] = group.normalizedMetrics
	}

	return frame
}
