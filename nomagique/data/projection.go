package data

import (
	"errors"
	"iter"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
)

/* MetricProjection names an explicit field; Defined gates optional evidence. */
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
ProjectionInput is the serialization boundary: named numbers and flags from a
completed observation, not a Primitive record graph.
*/
type ProjectionInput struct {
	Values map[string]float64
	Flags  map[string]bool
}

/*
Projection translates declared paths into the domain-facing measurement.
*/
type Projection struct {
	core.Base[ProjectionInput, *Measurement[float64]]
	Source    string
	Identity  func() (id string, label string, at time.Time, from time.Time)
	Metrics   []MetricProjection
	Facts     []FactProjection
	Accepted  []string
	Rejection error
}

func (op *Projection) Next(
	in iter.Seq[core.Primitive[ProjectionInput, ProjectionInput]],
) iter.Seq[core.Primitive[*Measurement[float64], *Measurement[float64]]] {
	return func(yield func(core.Primitive[*Measurement[float64], *Measurement[float64]]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(op.Project(arriving.Read()))) {
				return
			}
		}
	}
}

func (op *Projection) lookup(input ProjectionInput, path []string) (float64, bool) {
	if len(path) == 0 || input.Values == nil {
		return 0, false
	}

	value, ok := input.Values[path[0]]
	return value, ok
}

func (op *Projection) flag(input ProjectionInput, path []string) bool {
	if len(path) == 0 {
		return true
	}

	if input.Flags != nil {
		if value, ok := input.Flags[path[0]]; ok {
			return value
		}
	}

	return false
}

/* Project translates the declared record into the domain-facing measurement. */
func (op *Projection) Project(input ProjectionInput) *Measurement[float64] {
	var id, label string
	var at, from time.Time

	if op.Identity != nil {
		id, label, at, from = op.Identity()
	}

	measurement := NewMeasurement[float64](id, label, op.Source, at, from)

	if len(op.Accepted) != 0 && !op.flag(input, op.Accepted) {
		measurement.Err = op.Rejection

		if measurement.Err == nil {
			measurement.Err = errors.New("projection: observation not accepted")
		}

		return measurement
	}

	for _, metric := range op.Metrics {
		if len(metric.Defined) != 0 && !op.flag(input, metric.Defined) {
			continue
		}

		raw, _ := op.lookup(input, metric.Path)
		measurement.PutMetric(Metric[float64]{
			Label: metric.Label, Raw: raw, Unit: metric.Unit, Timescale: metric.Timescale,
		})
	}

	if len(op.Facts) != 0 {
		measurement.Metadata = make(map[string]float64, len(op.Facts))

		for _, fact := range op.Facts {
			if len(fact.Defined) != 0 && !op.flag(input, fact.Defined) {
				continue
			}

			if value, ok := op.lookup(input, fact.Path); ok {
				measurement.Metadata[fact.Name] = value
			}
		}
	}

	measurement.Finalize()
	return measurement
}
