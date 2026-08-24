package types

import (
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
MetricBinding defines how one interned nomagique Frame slot projects into a
domain Metric. The projector is the output boundary of a signal: it maps an
evaluated Frame into an immutable Measurement without any manual metric
boilerplate in the signal body.
*/
type MetricBinding struct {
	Symbol     nmtypes.Symbol
	Name       MetricType
	Side       MeasurementSide
	Normalized bool
	Unit       nmtypes.Unit
	Timescale  nmtypes.Timescale
}

/*
Projector declaratively maps an evaluated Frame into an immutable *Measurement.
*/
type Projector struct {
	source          SourceType
	bindings        []MetricBinding
	qualitySymbol   nmtypes.Symbol
	qualitySeparate bool
}

type ProjectorOption func(*Projector)

func NewProjector(source SourceType, opts ...ProjectorOption) *Projector {
	projector := &Projector{source: source}

	for _, opt := range opts {
		opt(projector)
	}

	return projector
}

/*
Metric binds a Frame slot to a standard raw domain Metric.
*/
func Metric(
	symbol nmtypes.Symbol,
	name MetricType,
	unit nmtypes.Unit,
	timescale nmtypes.Timescale,
	side ...MeasurementSide,
) ProjectorOption {
	measurementSide := SideNone

	if len(side) > 0 {
		measurementSide = side[0]
	}

	return func(projector *Projector) {
		projector.bindings = append(projector.bindings, MetricBinding{
			Symbol:     symbol,
			Name:       name,
			Side:       measurementSide,
			Unit:       unit,
			Timescale:  timescale,
		})
	}
}

/*
Normalized binds a Frame slot to a normalized [0, 1] domain Metric.
*/
func Normalized(
	symbol nmtypes.Symbol,
	name MetricType,
	unit nmtypes.Unit,
	timescale nmtypes.Timescale,
	side ...MeasurementSide,
) ProjectorOption {
	measurementSide := SideNone

	if len(side) > 0 {
		measurementSide = side[0]
	}

	return func(projector *Projector) {
		projector.bindings = append(projector.bindings, MetricBinding{
			Symbol:     symbol,
			Name:       name,
			Side:       measurementSide,
			Normalized:  true,
			Unit:       unit,
			Timescale:  timescale,
		})
	}
}

/*
Quality specifies which Frame symbol governs the hypothesis separation / quality
rating.
*/
func Quality(symbol nmtypes.Symbol) ProjectorOption {
	return func(projector *Projector) {
		projector.qualitySymbol = symbol
		projector.qualitySeparate = true
	}
}

/*
QualityRaw binds a Frame slot that is already a normalized [0,1] separation to
the quality rating without applying StandardSeparation.
*/
func QualityRaw(symbol nmtypes.Symbol) ProjectorOption {
	return func(projector *Projector) {
		projector.qualitySymbol = symbol
		projector.qualitySeparate = false
	}
}

/*
Project executes the projection from the evaluated Frame into a complete
Measurement, including provenance, metric bindings, and quality stamping.
*/
func (projector *Projector) Project(
	symbol string,
	at time.Time,
	observedFrom time.Time,
	output nmtypes.Frame,
) *nmtypes.Measurement {
	measurement := nmtypes.NewMeasurement(
		uuid.NewString(),
		string(projector.source),
		at.UnixNano(),
		observedFrom.UnixNano(),
	)
	measurement.Symbol = symbol
	measurement.At = at
	measurement.ObservedFrom = observedFrom

	if !observedFrom.IsZero() && at.After(observedFrom) {
		measurement.Horizon = at.Sub(observedFrom)
	}

	for _, binding := range projector.bindings {
		value, found := output.Get(binding.Symbol)

		if !found {
			continue
		}

		descriptor := nmtypes.Descriptor{
			Unit:      binding.Unit,
			Timescale: binding.Timescale,
		}
		key := MetricKey(binding.Name, binding.Side)

		if binding.Normalized {
			measurement.Put(key, nmtypes.NewNormalizedMetric(key, value, value, descriptor))
		} else {
			measurement.Put(key, nmtypes.NewMetric(key, value, descriptor))
		}
	}

	quality := 0.0

	if projector.qualitySymbol != 0 {
		if value, found := output.Get(projector.qualitySymbol); found {
			quality = value

			if projector.qualitySeparate {
				quality = statistic.StandardSeparation(value)
			}
		}
	}

	support, _ := output.Get(nmtypes.SampleCount)
	measurement.StampQuality(quality, support)

	return measurement
}
