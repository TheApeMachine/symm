package data

import (
	"fmt"
	"iter"
	"strconv"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
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
	*core.PrimitiveError

	finalizer core.Primitive
	Source    string
	Identity  func() (label string, at time.Time, from time.Time)
	Metrics   []MetricProjection
	Facts     []FactProjection
	Accepted  []string
	Rejection error
	out       *Measurement[float64]
}

/*
NewProjection creates a projection primitive for one declared record shape.
*/
func NewProjection(
	source string,
	identity func() (label string, at time.Time, from time.Time),
	metrics []MetricProjection,
	facts []FactProjection,
	accepted []string,
	rejection error,
) *Projection {
	return &Projection{PrimitiveError: core.NewPrimitiveError(), Source: source,
		Identity:  identity,
		Metrics:   metrics,
		Facts:     facts,
		Accepted:  accepted,
		Rejection: rejection,
	}
}

/*
Next translates each arriving record into the domain-facing measurement and
yields it.
*/
func (projection *Projection) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*ProjectionInput)(arriving)
			projection.out = projection.project(*input)

			if !yield(unsafe.Pointer(&projection.out)) {
				return
			}
		}
	}
}

/*
project translates the declared record into the domain-facing measurement.
*/
func (projection *Projection) project(input ProjectionInput) *Measurement[float64] {
	measurement := NewMeasurement(projection.Source, map[string]Metric[float64]{})

	if projection.Identity != nil {
		measurement.Label, measurement.At, measurement.From = projection.Identity()
	}

	if len(projection.Accepted) != 0 && !flagged(input, projection.Accepted) {
		measurement.Err = projection.Rejection

		if measurement.Err == nil {
			measurement.Err = fmt.Errorf("projection: observation not accepted")
		}

		return measurement
	}

	for _, metric := range projection.Metrics {
		if len(metric.Defined) != 0 && !flagged(input, metric.Defined) {
			continue
		}

		raw, _ := lookupPath(input, metric.Path)
		measurement.Metrics[metric.Label] = Metric[float64]{
			Label: metric.Label, Raw: raw, Unit: metric.Unit,
			Timescale: metric.Timescale,
		}
	}

	if len(projection.Facts) != 0 {
		measurement.Metadata = make(map[string]string, len(projection.Facts))

		for _, fact := range projection.Facts {
			if len(fact.Defined) != 0 && !flagged(input, fact.Defined) {
				continue
			}

			if value, ok := lookupPath(input, fact.Path); ok {
				measurement.Metadata[fact.Name] = strconv.FormatFloat(value, 'f', -1, 64)
			}
		}
	}

	if projection.finalizer == nil {
		projection.finalizer = NewFinalizer[float64]()
	}

	for range projection.finalizer.Next(sequence.NewValues(measurement).Next(nil)) {
	}

	return measurement
}

/*
lookupPath reads one declared path from the record's named numbers.
*/
func lookupPath(input ProjectionInput, path []string) (float64, bool) {
	if len(path) == 0 || input.Values == nil {
		return 0, false
	}

	value, ok := input.Values[path[0]]
	return value, ok
}

/*
flagged reads one declared flag from the record; an absent path list passes.
*/
func flagged(input ProjectionInput, path []string) bool {
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
