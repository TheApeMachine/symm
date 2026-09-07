package data

import (
	"errors"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* MetricProjection names an explicit graph field; Defined gates optional evidence. */
type MetricProjection struct {
	Label         string
	Path, Defined []string
	Unit          Unit
	Timescale     Timescale
}

/* FactProjection declares estimator metadata and its optional evidence gate. */
type FactProjection struct {
	Name          string
	Path, Defined []string
}

/*
Projection is a serialization boundary over an explicit Primitive result record.
It never inspects upstream receivers. Metrics and estimator facts are declared
by the graph's composition root, not inferred from legacy Reporter interfaces.
*/
type Projection struct {
	core.PrimitiveError
	Source      string
	Identity    func() (id string, label string, at time.Time, from time.Time)
	Metrics     []MetricProjection
	Facts       []FactProjection
	Accepted    []string
	Rejection   error
	seed        core.Primitive
	measurement *Measurement[float64]
}

func (projection *Projection) Next(input core.Primitive) core.Primitive {
	if projection.seed == nil {
		projection.seed = transport.NewIO(core.From((*Measurement[float64])(nil)))
	}
	return core.Yield(projection.seed, input,
		func(_ *Measurement[float64], fields map[string]core.Primitive) *Measurement[float64] {
			projection.measurement = projection.Project(fields)
			return projection.measurement
		}, projection)
}

func (projection *Projection) Read() any { return projection.measurement }

/* Project translates the declared record into the domain-facing measurement. */
func (projection *Projection) Project(fields map[string]core.Primitive) *Measurement[float64] {
	var id, label string
	var at, from time.Time
	if projection.Identity != nil {
		id, label, at, from = projection.Identity()
	}
	measurement := NewMeasurement[float64](id, label, projection.Source, at, from)
	decoder := core.NewDecoder(fields)
	if len(projection.Accepted) != 0 && !core.Decode[bool](decoder, projection.Accepted...) {
		measurement.Err = errors.Join(projection.Rejection, decoder.Error())
		if measurement.Err == nil {
			measurement.Err = errors.New("projection: observation not accepted")
		}
		return measurement
	}
	for _, metric := range projection.Metrics {
		if len(metric.Defined) != 0 && !core.Decode[bool](decoder, metric.Defined...) {
			continue
		}
		measurement.PutMetric(Metric[float64]{Label: metric.Label,
			Raw: core.Decode[float64](decoder, metric.Path...), Unit: metric.Unit, Timescale: metric.Timescale})
	}
	if len(projection.Facts) != 0 {
		measurement.Metadata = make(map[string]float64, len(projection.Facts))
		for _, fact := range projection.Facts {
			if len(fact.Defined) != 0 && !core.Decode[bool](decoder, fact.Defined...) {
				continue
			}
			measurement.Metadata[fact.Name] = core.Decode[float64](decoder, fact.Path...)
		}
	}
	measurement.Err = decoder.Error()
	measurement.Finalize()
	return measurement
}

var _ core.Primitive = (*Projection)(nil)
