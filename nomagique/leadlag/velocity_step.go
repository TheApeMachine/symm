package leadlag

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
VelocityStep owns two temporal velocities — lag and correlation gain —
and stamps each velocity's rate onto the measurement when defined.
*/
type VelocityStep struct {
	*core.PrimitiveError
	lagVel  core.Primitive
	gainVel core.Primitive
}

func NewVelocityStep() core.Primitive {
	return &VelocityStep{
		PrimitiveError: core.NewPrimitiveError(),
		lagVel:         temporal.NewVelocity(),
		gainVel:        temporal.NewVelocity(),
	}
}

func (op *VelocityStep) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			at := m.At.UnixNano()

			lagObs := temporal.Observation{Value: m.Metrics["best_lag_seconds"].Raw, At: at}
			gainObs := temporal.Observation{Value: m.Metrics["absolute_correlation_gain"].Raw, At: at}

			var lagReading temporal.VelocityReading

			for out := range op.lagVel.Next(transport.NewOne(unsafe.Pointer(&lagObs)).Next(nil)) {
				lagReading = *(*temporal.VelocityReading)(out)
			}

			var gainReading temporal.VelocityReading

			for out := range op.gainVel.Next(transport.NewOne(unsafe.Pointer(&gainObs)).Next(nil)) {
				gainReading = *(*temporal.VelocityReading)(out)
			}

			if lagReading.Defined {
				m.Metrics["lag_velocity"] = m.Metrics["lag_velocity"].Write(lagReading.Rate)
			}

			if gainReading.Defined {
				m.Metrics["correlation_gain_velocity"] = m.Metrics["correlation_gain_velocity"].Write(gainReading.Rate)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
