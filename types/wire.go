package types

import (
	"time"

	"github.com/theapemachine/datura"
)

/*
wireGroup is one source×symbol observation assembled for the compact UI wire.
*/
type wireGroup struct {
	source   SourceType
	stream   StreamType
	symbol   string
	at       time.Time
	maturity float64
	validity MeasurementValidity
	metrics  datura.Map[any]
}

/*
WireMeasurements collapses flat typed measurement rows into one compact map per
source×symbol observation. Metric names become keys under metrics; directional
Hawkes values that share a metric name keep their side as metric:side so buy and
sell intensities are not clobbered. Thesis and evidence graphs keep the flat
typed rows; only the UI publish path uses this projection.
*/
func WireMeasurements(measurements []*Measurement) []datura.Map[any] {
	latest := FilterLatest(measurements)

	if len(latest) == 0 {
		return nil
	}

	groups := make(map[string]*wireGroup, len(latest))
	order := make([]string, 0, len(latest))

	for _, measurement := range latest {
		if measurement == nil || measurement.Symbol == "" || measurement.Metric == "" {
			continue
		}

		key := string(measurement.Source) + "\x00" + measurement.Symbol
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
newWireGroup seeds one source×symbol observation from its first measurement.
*/
func newWireGroup(measurement *Measurement) *wireGroup {
	return &wireGroup{
		source:   measurement.Source,
		stream:   measurement.Stream,
		symbol:   measurement.Symbol,
		at:       measurement.At,
		maturity: measurement.Maturity,
		validity: measurement.Validity,
		metrics:  datura.Map[any]{},
	}
}

/*
accumulate merges one metric row and advances observation metadata when newer.
*/
func (group *wireGroup) accumulate(measurement *Measurement) {
	if measurement.At.After(group.at) {
		group.at = measurement.At
		group.maturity = measurement.Maturity
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

	seen := make(map[string]struct{}, len(latest))

	for _, measurement := range latest {
		if measurement == nil || measurement.Symbol == "" || measurement.Metric == "" {
			continue
		}

		seen[string(measurement.Source)+"\x00"+measurement.Symbol] = struct{}{}
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
