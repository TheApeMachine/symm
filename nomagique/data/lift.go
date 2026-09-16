package data

import (
	"iter"
	"strings"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
LiftReading is one observation flattened into values keyed by source-qualified
metric name, resolving metric values through their authority so no naked
unconditioned numbers escape into the system.

A metric's identity is its source and its own label — "hawkes/arrival_rate" —
so measurements from different signals never collide and a consumer names the
evidence it wants without knowing which measurement carried it.
*/
type LiftReading struct {
	Values map[string]float64
}

/*
ReadoutLift is one observation flattened into Readouts keyed by
source-qualified metric name, preserving the full quality context (maturity,
SNR, credibility, and corroborations) for downstream logic layers.
*/
type ReadoutLift struct {
	Readouts map[string]Readout
}

/*
Lift folds a run of measurements into one authority-conditioned observation,
yielding the observation after every arrival.

A measurement carrying an Err is skipped rather than discarded: one failed
signal must not erase every other signal's metrics from the same observation.
The error is recorded through Error so a caller can report it, but the metrics
that were successfully measured still reach the observation.
*/
type Lift struct {
	*core.PrimitiveError

	authority core.Primitive
	out       LiftReading
}

/*
NewLift creates the measurement flattening primitive.
*/
func NewLift() *Lift {
	return &Lift{PrimitiveError: core.NewPrimitiveError(), authority: NewAuthority()}
}

func (lift *Lift) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			measurement := *(**Measurement[float64])(arriving)

			if measurement == nil {
				continue
			}

			if lift.out.Values == nil {
				lift.out.Values = make(map[string]float64)
			}

			if measurement.Err != nil {
				lift.Error(measurement.Err)
				continue
			}

			var authority float64

			for out := range lift.authority.Next(sequence.NewValues(qualityOf(measurement)).Next(nil)) {
				authority = *(*float64)(out)
			}

			if err := lift.authority.Error(); err != nil {
				lift.Error(err)
				return
			}

			for label, metric := range measurement.Metrics {
				if metric.Unit == UnitCount || strings.Contains(label, "ordinal") {
					lift.out.Values[measurement.Source+"/"+label] = metric.Raw
					continue
				}

				lift.out.Values[measurement.Source+"/"+label] = metric.Raw * authority
			}

			if !yield(unsafe.Pointer(&lift.out)) {
				return
			}
		}
	}
}

/*
LiftReadouts folds a run of measurements into Readouts keyed by
source-qualified metric name, yielding the collection after every arrival.
Failed measurements are skipped with their error recorded, never silently
discarded.
*/
type LiftReadouts struct {
	*core.PrimitiveError

	finalizer core.Primitive
	readout   core.Primitive
	out       ReadoutLift
}

/*
NewLiftReadouts creates the measurement readout flattening primitive.
*/
func NewLiftReadouts() *LiftReadouts {
	return &LiftReadouts{PrimitiveError: core.NewPrimitiveError(), finalizer: NewFinalizer[float64](),
		readout: NewReadout(),
	}
}

func (liftReadouts *LiftReadouts) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			measurement := *(**Measurement[float64])(arriving)

			if measurement == nil {
				continue
			}

			if liftReadouts.out.Readouts == nil {
				liftReadouts.out.Readouts = make(map[string]Readout)
			}

			if measurement.Err != nil {
				liftReadouts.Error(measurement.Err)
				continue
			}

			if measurement.Maturity == 0 && !measurement.SNRDefined && !measurement.Estimated {
				for range liftReadouts.finalizer.Next(sequence.NewOne(arriving).Next(nil)) {
				}

				if err := liftReadouts.finalizer.Error(); err != nil {
					liftReadouts.Error(err)
					return
				}
			}

			if measurement.Err != nil {
				continue
			}

			for label, metric := range measurement.Metrics {
				raw, valid := any(metric.Raw).(float64)

				if !valid {
					continue
				}

				readingEval := liftReadouts.readout
				var reading Readout

				for out := range readingEval.Next(sequence.NewValues(ReadoutInput{
					QualityReading: qualityOf(measurement),
					Raw:            raw,
					Credibility:    1,
					Defined:        true,
					Discrete: metric.Unit == UnitCount ||
						strings.Contains(label, "ordinal"),
				}).Next(nil)) {
					reading = *(*Readout)(out)
				}

				err := readingEval.Error()

				if err != nil {
					liftReadouts.Error(err)
					continue
				}

				liftReadouts.out.Readouts[measurement.Source+"/"+label] = reading
			}

			if !yield(unsafe.Pointer(&liftReadouts.out)) {
				return
			}
		}
	}
}
