package learning

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
PaceConfig is rest, bounds, gain, band, and window. These are explicit
configuration, not newly chosen tuning constants.
*/
type PaceConfig struct {
	Rest   float64
	Lower  float64
	Upper  float64
	Gain   float64
	Band   float64
	Window float64
}

/*
PaceReading is the adapted learning pace and the current error rank.
*/
type PaceReading struct {
	Alpha float64
	Rank  float64
	Ready bool
	Count float64
}

/*
Pace owns empirical prior-error rank and the bounded log-alpha controller.
The calibrator owns history; Mix owns movement.
*/
type Pace struct {
	err        error
	config     PaceConfig
	calibrator core.Primitive
	mix        core.Primitive
	bound      core.Primitive
	logAlpha   float64
	alpha      float64
	seeded     bool
	out        PaceReading
}

func NewPace(config PaceConfig) core.Primitive {
	return &Pace{
		config:     config,
		calibrator: probability.NewCalibrator(collection.NewTail[float64](int(config.Window))),
		mix:        calculus.NewMix(),
		bound:      calculus.NewBound(),
	}
}

func (op *Pace) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)

			if err := op.validate(); err != nil {
				op.Error(err)
				return
			}

			if math.IsNaN(val) || math.IsInf(val, 0) {
				op.Error(core.ErrDomain)
				return
			}

			if !op.seeded {
				op.logAlpha = math.Log(op.config.Rest)
				op.alpha = op.config.Rest
				op.seeded = true
			}

			var calibration probability.CalibratorReading

			for out := range op.calibrator.Next(transport.NewValues(val).Next(nil)) {
				calibration = *(*probability.CalibratorReading)(out)
			}

			if err := op.calibrator.Error(); err != nil {
				op.Error(err)
				return
			}

			ready := op.config.Window <= calibration.PriorCount
			count := math.Min(op.config.Window, calibration.PriorCount+1)
			rank := 0.0

			if ready {
				rank = calibration.Value
				logRest := math.Log(op.config.Rest)
				logMin := math.Log(op.config.Lower)
				logMax := math.Log(op.config.Upper)

				target := logRest

				if rank < op.config.Band {
					target = logMax
				}

				if rank > 1-op.config.Band {
					target = logMin
				}

				mixRec := calculus.MixRecord{
					Left:   op.logAlpha,
					Right:  target,
					Weight: op.config.Gain,
				}

				var mixed float64

				for out := range op.mix.Next(transport.NewValues(mixRec).Next(nil)) {
					mixed = *(*float64)(out)
				}

				if err := op.mix.Error(); err != nil {
					op.Error(err)
					return
				}

				boundRec := calculus.BoundRecord{
					Value: mixed,
					Lower: logMin,
					Upper: logMax,
				}

				var logAlpha float64

				for out := range op.bound.Next(transport.NewValues(boundRec).Next(nil)) {
					logAlpha = *(*float64)(out)
				}

				if err := op.bound.Error(); err != nil {
					op.Error(err)
					return
				}

				alpha := math.Exp(logAlpha)
				boundAlpha := calculus.BoundRecord{
					Value: alpha,
					Lower: op.config.Lower,
					Upper: op.config.Upper,
				}

				for out := range op.bound.Next(transport.NewValues(boundAlpha).Next(nil)) {
					alpha = *(*float64)(out)
				}

				if err := op.bound.Error(); err != nil {
					op.Error(err)
					return
				}

				op.logAlpha = logAlpha
				op.alpha = alpha
			}

			op.out = PaceReading{
				Alpha: op.alpha,
				Rank:  rank,
				Ready: ready,
				Count: count,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Pace) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

func (op *Pace) validate() error {
	for _, value := range []float64{
		op.config.Rest, op.config.Lower, op.config.Upper,
		op.config.Gain, op.config.Band, op.config.Window,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return core.ErrDomain
		}
	}

	if !(op.config.Lower > 0) ||
		!(op.config.Lower <= op.config.Rest) ||
		!(op.config.Rest <= op.config.Upper) ||
		!(op.config.Window > 0) ||
		math.Floor(op.config.Window) != op.config.Window {
		return core.ErrDomain
	}

	return nil
}
