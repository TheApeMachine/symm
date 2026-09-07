package temporal

import (
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Velocity owns the previous observation and its configured value/clock sources. */
type Velocity struct {
	*transport.Map
	Reading VelocityReading
}

/* VelocityPoint retains a value at its exact nanosecond coordinate. */
type VelocityPoint struct {
	Value float64
	At    int64
}

/* VelocityReading fixes a finite difference, including its definedness. */
type VelocityReading struct {
	core.PrimitiveError
	From, Through             VelocityPoint
	Elapsed, Difference, Rate float64
	HasPrior, Defined         bool
	observed                  bool
}

/*
NewVelocity evaluates source and clock once per input. The first observation and
non-advancing time have zero rate with explicit definedness. The latest point is
always retained, including when its clock does not advance.
*/
func NewVelocity(source, clock core.Primitive) *Velocity {
	velocity := &Velocity{}
	velocity.Map = transport.NewMap(transport.NewPipe(
		transport.NewZip(source, clock),
		&velocityStep{velocity: velocity, seed: transport.NewIO(&VelocityReading{})},
	))
	return velocity
}

/* Observe advances the finite difference without boxing numeric operations. */
func (velocity *Velocity) Observe(value float64, at int64) VelocityReading {
	reading := VelocityReading{
		Through:  VelocityPoint{Value: value, At: at},
		HasPrior: velocity.Reading.observed, observed: true,
	}

	if reading.HasPrior {
		reading.From = velocity.Reading.Through
		reading.Elapsed = float64(at-reading.From.At) / float64(time.Second)
		reading.Difference = value - reading.From.Value
		reading.Defined = reading.Elapsed > 0
	}

	if reading.Defined {
		reading.Rate = reading.Difference / reading.Elapsed
	}
	velocity.Reading = reading
	return reading
}

/* Read materializes a named record only at a Primitive boundary. */
func (reading *VelocityReading) Read() any {
	fields := map[string]core.Primitive{
		"through": core.Record(map[string]any{"at": reading.Through.At, "value": reading.Through.Value}),
		"elapsed": core.From(reading.Elapsed), "difference": core.From(reading.Difference),
		"rate": core.From(reading.Rate), "has_prior": core.From(reading.HasPrior),
		"defined": core.From(reading.Defined),
	}

	if reading.HasPrior {
		fields["from"] = core.Record(map[string]any{"at": reading.From.At, "value": reading.From.Value})
	}
	return fields
}

func (reading *VelocityReading) Next(core.Primitive) core.Primitive { return nil }

/* velocityStep adapts paired source delivery to the owned finite difference. */
type velocityStep struct {
	core.PrimitiveError
	velocity *Velocity
	seed     *transport.IO
}

func (step *velocityStep) Next(input core.Primitive) core.Primitive {
	return core.Yield(step.seed, input,
		func(_ core.Primitive, values []core.Primitive) core.Primitive {
			value, at := core.To[float64](values[0]), core.To[int64](values[1])
			step.Error(values[0].Error(), values[1].Error())

			if step.Error() != nil {
				return nil
			}
			reading := step.velocity.Observe(value, at)
			return &reading
		}, step)
}

func (step *velocityStep) Read() any { return step.velocity.Reading.Read() }
