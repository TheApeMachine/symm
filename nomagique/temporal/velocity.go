package temporal

import (
	"iter"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Observation is a value at its nanosecond coordinate. Pairing of source and
clock is external: Velocity sees one observation.
*/
type Observation struct {
	Value float64
	At    int64
}

/*
VelocityPoint retains a value at its exact nanosecond coordinate.
*/
type VelocityPoint struct {
	Value float64
	At    int64
}

/*
VelocityReading fixes a finite difference, including its definedness.
*/
type VelocityReading struct {
	From, Through             VelocityPoint
	Elapsed, Difference, Rate float64
	HasPrior, Defined         bool
	observed                  bool
}

/*
Velocity owns the previous observation. The first observation and
non-advancing time have zero rate with explicit definedness. The latest point
is always retained, including when its clock does not advance.
*/
type Velocity struct {
	core.Base[Observation, VelocityReading]
	Reading VelocityReading
}

func NewVelocity() *Velocity {
	return &Velocity{}
}

func (op *Velocity) Next(
	in iter.Seq[core.Primitive[Observation, Observation]],
) iter.Seq[core.Primitive[VelocityReading, VelocityReading]] {
	return func(yield func(core.Primitive[VelocityReading, VelocityReading]) bool) {
		for arriving := range in {
			point := arriving.Read()
			reading := op.Observe(point.Value, point.At)

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}

/*
Observe advances the finite difference without boxing numeric operations.
*/
func (op *Velocity) Observe(value float64, at int64) VelocityReading {
	reading := VelocityReading{
		Through:  VelocityPoint{Value: value, At: at},
		HasPrior: op.Reading.observed,
		observed: true,
	}

	if reading.HasPrior {
		reading.From = op.Reading.Through
		reading.Elapsed = float64(at-reading.From.At) / float64(time.Second)
		reading.Difference = value - reading.From.Value
		reading.Defined = reading.Elapsed > 0
	}

	if reading.Defined {
		reading.Rate = reading.Difference / reading.Elapsed
	}

	op.Reading = reading
	return reading
}
