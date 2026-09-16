package temporal

import (
	"iter"
	"time"
	"unsafe"

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
	*core.PrimitiveError

	reading VelocityReading
}

func NewVelocity() *Velocity {
	return &Velocity{PrimitiveError: core.NewPrimitiveError()}
}

func (velocity *Velocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			point := (*Observation)(arriving)
			reading := VelocityReading{
				Through:  VelocityPoint{Value: point.Value, At: point.At},
				HasPrior: velocity.reading.observed,
				observed: true,
			}

			if reading.HasPrior {
				reading.From = velocity.reading.Through
				reading.Elapsed = float64(point.At-reading.From.At) / float64(time.Second)
				reading.Difference = point.Value - reading.From.Value
				reading.Defined = reading.Elapsed > 0
			}

			if reading.Defined {
				reading.Rate = reading.Difference / reading.Elapsed
			}

			velocity.reading = reading

			if !yield(unsafe.Pointer(&velocity.reading)) {
				return
			}
		}
	}
}
