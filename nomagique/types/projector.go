package types

import (
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/symm/nomagique/statistic"
)

// MetricBinding defines how one interned Frame slot projects into a domain Metric.
type MetricBinding struct {
	Symbol     Symbol
	Name       MetricType
	Side       MeasurementSide
	Normalized bool
	Unit       nmtypes.Unit
	Timescale  nmtypes.Timescale
}

// Projector declaratively maps an evaluated Frame into an immutable *Measurement.
type Projector struct {
	source        SourceType
	bindings      []MetricBinding
	qualitySymbol nmtypes.Symbol
}

type ProjectorOption func(*Projector)

func NewProjector(source SourceType, opts ...ProjectorOption) *Projector {
	p := &Projector{source: source}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Metric binds a Frame slot to a standard raw domain Metric.
func Metric(sym nmtypes.Symbol, name MetricType, unit nmtypes.Unit, timescale nmtypes.Timescale, side ...MeasurementSide) ProjectorOption {
	s := SideNone
	if len(side) > 0 {
		s = side[0]
	}
	return func(p *Projector) {
		p.bindings = append(p.bindings, MetricBinding{
			Symbol: sym, Name: name, Side: s, Unit: unit, Timescale: timescale,
		})
	}
}

// Normalized binds a Frame slot to a normalized [0, 1] domain Metric.
func Normalized(sym nmtypes.Symbol, name MetricType, unit nmtypes.Unit, timescale nmtypes.Timescale, side ...MeasurementSide) ProjectorOption {
	s := SideNone
	if len(side) > 0 {
		s = side[0]
	}
	return func(p *Projector) {
		p.bindings = append(p.bindings, MetricBinding{
			Symbol: sym, Name: name, Side: s, Normalized: true, Unit: unit, Timescale: timescale,
		})
	}
}

// Quality specifies which Frame symbol governs the hypothesis separation / quality rating.
func Quality(sym nmtypes.Symbol) ProjectorOption {
	return func(p *Projector) {
		p.qualitySymbol = sym
	}
}

// Project executes the projection from the evaluated Frame into a complete Measurement.
func (p *Projector) Project(symbol string, at time.Time, frame nmtypes.Frame) *nmtypes.Measurement {
	m := nmtypes.NewMeasurement(uuid.NewString(), string(p.source), at.UnixNano(), at.UnixNano())
	m.Symbol = symbol

	for _, b := range p.bindings {
		val, found := frame.Get(b.Symbol)
		if !found {
			continue
		}

		desc := nmtypes.Descriptor{Unit: b.Unit, Timescale: b.Timescale}
		key := MetricKey(b.Name, b.Side)

		if b.Normalized {
			m.Put(key, nmtypes.NewNormalizedMetric(key, val, val, desc))
		} else {
			m.Put(key, nmtypes.NewMetric(key, val, desc))
		}
	}

	// Automatic quality separation & empirical support calculation
	quality := 0.0
	if p.qualitySymbol != 0 {
		if q, found := frame.Get(p.qualitySymbol); found {
			quality = statistic.StandardSeparation(q)
		}
	}
	support, _ := frame.Get(nmtypes.SampleCount)
	m.StampQuality(quality, support)

	return m
}
