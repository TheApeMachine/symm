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
	source   SourceType
	stream   StreamType
	symbol   string
	at       time.Time
	maturity float64
	validity MeasurementValidity
	scale    ScaleReference
	metrics  datura.Map[any]
}

/*
WireMeasurements collapses flat typed measurement rows into one compact map per
source×symbol×at observation. Metric names become keys under metrics; directional
Hawkes values that share a metric name keep their side as metric:side so buy and
sell intensities are not clobbered. Fit-parameter epochs keep their own at and
scale beside the live intensity epoch so the UI can reconstruct decay curves.
Thesis and evidence graphs keep the flat typed rows; only the UI publish path
uses this projection.
*/
func WireMeasurements(measurements []*Measurement) []datura.Map[any] {
	candidates := wireCandidates(measurements)

	if len(candidates) == 0 {
		return nil
	}

	groups := make(map[wireKey]*wireGroup, len(candidates))
	order := make([]wireKey, 0, len(candidates))

	for _, measurement := range candidates {
		if measurement == nil || measurement.Symbol == "" || measurement.Metric == "" {
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

/*
wireCandidates keeps the newest complete epoch per symbol and any older
fit-parameter rows that FilterLatest would otherwise drop.
*/
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
		if measurement == nil || !wireFitParameter(measurement) {
			continue
		}

		if _, exists := latestKeys[wireGroupKey(measurement)]; exists {
			continue
		}

		out = append(out, measurement)
	}

	return out
}

/*
wireGroupKey identifies one wire observation slot by source, symbol, and at.
*/
func wireGroupKey(measurement *Measurement) wireKey {
	return wireKey{
		source: measurement.Source,
		symbol: measurement.Symbol,
		atNano: measurement.At.UTC().UnixNano(),
	}
}

/*
wireFitParameter reports whether a measurement carries retained Hawkes fit
parameters that must travel beside newer evaluation intensities.
*/
func wireFitParameter(measurement *Measurement) bool {
	switch measurement.Metric {
	case MetricBaselineIntensity,
		MetricExcitationAmplitude,
		MetricDecayRate,
		MetricKernelMemory,
		MetricSpectralRadius,
		MetricHawkesPoissonDelta,
		MetricCrossSelfDelta,
		MetricImmediateOffspring,
		MetricTotalDescendants:
		return true
	default:
		return false
	}
}

/*
newWireGroup seeds one source×symbol×at observation from its first measurement.
*/
func newWireGroup(measurement *Measurement) *wireGroup {
	return &wireGroup{
		source:   measurement.Source,
		stream:   measurement.Stream,
		symbol:   measurement.Symbol,
		at:       measurement.At,
		maturity: measurement.Maturity,
		validity: measurement.Validity,
		scale:    measurement.Scale,
		metrics:  make(datura.Map[any], 8),
	}
}

/*
accumulate merges one metric row into a fixed-at observation group.
*/
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

	group.metrics[wireMetricKey(measurement)] = measurement.Raw
}

/*
frame encodes one wire observation map for the UI publish path.
*/
func (group *wireGroup) frame() datura.Map[any] {
	frame := datura.Map[any]{
		"source":  group.source,
		"symbol":  group.symbol,
		"at":      group.at,
		"metrics": group.metrics,
	}

	if group.stream != "" {
		frame["stream"] = group.stream
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

	return frame
}

/*
ObservationCount reports how many source×symbol wire observations a flat
measurement batch represents, matching the dashboard tick counter without
allocating the nested wire maps.
*/
func ObservationCount(measurements []*Measurement) int {
	latest := FilterLatest(measurements)

	if len(latest) == 0 {
		return 0
	}

	type pair struct {
		source SourceType
		symbol string
	}

	seen := make(map[pair]struct{}, len(latest))

	for _, measurement := range latest {
		if measurement == nil || measurement.Symbol == "" || measurement.Metric == "" {
			continue
		}

		seen[pair{measurement.Source, measurement.Symbol}] = struct{}{}
	}

	return len(seen)
}

/*
wireMetricKey names one metric on the wire. Empty side keeps the bare metric
name; a non-empty side appends :side so directional values remain distinct.
*/
func wireMetricKey(measurement *Measurement) string {
	if measurement.Side == SideNone || measurement.Side == "" {
		return string(measurement.Metric)
	}

	return string(measurement.Metric) + ":" + string(measurement.Side)
}
