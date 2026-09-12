package correlation

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Fold folds the admitted peers into one cohort summary and derives the
focal-to-cohort relative return energy rate. Without admitted peers there is
nothing to fold and the reading moves through untouched.
*/
type Fold struct {
	err    error
	cohort core.Primitive
}

func NewFold() core.Primitive {
	return &Fold{cohort: NewCohort()}
}

func (op *Fold) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*Reading)(arriving)

			if len(reading.Admitted) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			summary := fold(op.cohort, reading.Admitted)

			if err := op.cohort.Error(); err != nil {
				op.Error(err)

				return
			}

			reading.Cohort = summary

			if summary.PeerEnergyRate > 0 {
				reading.Relative = reading.Selected.Dependence.LeftEnergyRate / summary.PeerEnergyRate
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Fold) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
History feeds the cohort's signed correlation to the Fisher-space causal
estimator, giving the reading its baseline, divergence, and z-score.
*/
type History struct {
	err       error
	estimator core.Primitive
}

func NewHistory() core.Primitive {
	return &History{estimator: NewFisherEstimator()}
}

func (op *History) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*Reading)(arriving)

			if len(reading.Admitted) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			view := drive[float64, FisherView](op.estimator, &reading.Cohort.SignedCorrelation)
			reading.History = view

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *History) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
Relative tracks the focal-to-cohort relative return energy rate against its
own adaptive baseline.
*/
type Relative struct {
	err      error
	baseline core.Primitive
}

func NewRelative() core.Primitive {
	return &Relative{baseline: adaptive.NewBaseline(adaptive.NewWindow())}
}

func (op *Relative) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*Reading)(arriving)

			if len(reading.Admitted) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			reading.RelativeHistory = drive[float64, adaptive.BaselineReading](op.baseline, &reading.Relative)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Relative) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
CorrelationVelocity measures how fast the cohort's signed correlation moves.
*/
type CorrelationVelocity struct {
	err      error
	velocity core.Primitive
}

func NewCorrelationVelocity() core.Primitive {
	return &CorrelationVelocity{velocity: temporal.NewVelocity()}
}

func (op *CorrelationVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*Reading)(arriving)

			if len(reading.Admitted) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			observation := temporal.Observation{Value: reading.Cohort.SignedCorrelation, At: reading.At}
			reading.CorrelationVelocity = drive[temporal.Observation, temporal.VelocityReading](op.velocity, &observation)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *CorrelationVelocity) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
EnergyVelocity measures how fast the relative return energy rate moves.
*/
type EnergyVelocity struct {
	err      error
	velocity core.Primitive
}

func NewEnergyVelocity() core.Primitive {
	return &EnergyVelocity{velocity: temporal.NewVelocity()}
}

func (op *EnergyVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*Reading)(arriving)

			if len(reading.Admitted) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			observation := temporal.Observation{Value: reading.Relative, At: reading.At}
			reading.EnergyVelocity = drive[temporal.Observation, temporal.VelocityReading](op.velocity, &observation)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *EnergyVelocity) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
fold pushes one peer run through the cohort reduction and returns the summary.
*/
func fold(op core.Primitive, peers []Peer) CohortSummary {
	var summary CohortSummary

	for out := range op.Next(transport.NewValues(peers...).Next(nil)) {
		summary = *(*CohortSummary)(out)
	}

	return summary
}
