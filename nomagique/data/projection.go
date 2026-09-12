package data

import (
	"errors"
	"fmt"
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
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
	err       error
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
) core.Primitive {
	return &Projection{
		Source:    source,
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
func (op *Projection) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*ProjectionInput)(arriving)
			op.out = op.project(*input)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Projection) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
project translates the declared record into the domain-facing measurement.
*/
func (op *Projection) project(input ProjectionInput) *Measurement[float64] {
	measurement := NewMeasurement[float64](op.Source, map[string]Metric[float64]{})

	if op.Identity != nil {
		measurement.Label, measurement.At, measurement.From = op.Identity()
	}

	if len(op.Accepted) != 0 && !flagged(input, op.Accepted) {
		measurement.Err = op.Rejection

		if measurement.Err == nil {
			measurement.Err = fmt.Errorf("projection: observation not accepted")
		}

		return measurement
	}

	for _, metric := range op.Metrics {
		if len(metric.Defined) != 0 && !flagged(input, metric.Defined) {
			continue
		}

		raw, _ := lookupPath(input, metric.Path)
		measurement.Metrics[metric.Label] = Metric[float64]{
			Label: metric.Label, Raw: raw, Unit: metric.Unit,
			Timescale: metric.Timescale,
		}
	}

	if len(op.Facts) != 0 {
		measurement.Metadata = make(map[string]float64, len(op.Facts))

		for _, fact := range op.Facts {
			if len(fact.Defined) != 0 && !flagged(input, fact.Defined) {
				continue
			}

			if value, ok := lookupPath(input, fact.Path); ok {
				measurement.Metadata[fact.Name] = value
			}
		}
	}

	if op.finalizer == nil {
		op.finalizer = NewFinalizer[float64]()
	}

	for range op.finalizer.Next(transport.NewValues(measurement).Next(nil)) {
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
