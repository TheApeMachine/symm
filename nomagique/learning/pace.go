package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/probability"
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
	*core.PrimitiveError

	config     PaceConfig
	calibrator core.Primitive
	mix        core.Primitive
	bound      core.Primitive
	logAlpha   float64
	alpha      float64
	seeded     bool
	out        PaceReading
}

func NewPace(config PaceConfig) *Pace {
	return &Pace{PrimitiveError: core.NewPrimitiveError(), config: config,
		calibrator: probability.NewCalibrator(sequence.NewTail[float64](int(config.Window))),
		mix:        calculus.NewMix(),
		bound:      calculus.NewBound(),
	}
}

func (pace *Pace) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)

			if err := pace.validate(); err != nil {
				pace.Error(err)
				return
			}

			if math.IsNaN(val) || math.IsInf(val, 0) {
				pace.Error(core.ErrDomain)
				return
			}

			if !pace.seeded {
				pace.logAlpha = math.Log(pace.config.Rest)
				pace.alpha = pace.config.Rest
				pace.seeded = true
			}

			var calibration probability.CalibratorReading

			for out := range pace.calibrator.Next(sequence.NewValues(val).Next(nil)) {
				calibration = *(*probability.CalibratorReading)(out)
			}

			if err := pace.calibrator.Error(); err != nil {
				pace.Error(err)
				return
			}

			ready := pace.config.Window <= calibration.PriorCount
			count := math.Min(pace.config.Window, calibration.PriorCount+1)
			rank := 0.0

			if ready {
				rank = calibration.Value
				logRest := math.Log(pace.config.Rest)
				logMin := math.Log(pace.config.Lower)
				logMax := math.Log(pace.config.Upper)

				target := logRest

				if rank < pace.config.Band {
					target = logMax
				}

				if rank > 1-pace.config.Band {
					target = logMin
				}

				mixRec := calculus.MixRecord{
					Left:   pace.logAlpha,
					Right:  target,
					Weight: pace.config.Gain,
				}

				var mixed float64

				for out := range pace.mix.Next(sequence.NewValues(mixRec).Next(nil)) {
					mixed = *(*float64)(out)
				}

				if err := pace.mix.Error(); err != nil {
					pace.Error(err)
					return
				}

				boundRec := calculus.BoundRecord{
					Value: mixed,
					Lower: logMin,
					Upper: logMax,
				}

				var logAlpha float64

				for out := range pace.bound.Next(sequence.NewValues(boundRec).Next(nil)) {
					logAlpha = *(*float64)(out)
				}

				if err := pace.bound.Error(); err != nil {
					pace.Error(err)
					return
				}

				alpha := math.Exp(logAlpha)
				boundAlpha := calculus.BoundRecord{
					Value: alpha,
					Lower: pace.config.Lower,
					Upper: pace.config.Upper,
				}

				for out := range pace.bound.Next(sequence.NewValues(boundAlpha).Next(nil)) {
					alpha = *(*float64)(out)
				}

				if err := pace.bound.Error(); err != nil {
					pace.Error(err)
					return
				}

				pace.logAlpha = logAlpha
				pace.alpha = alpha
			}

			pace.out = PaceReading{
				Alpha: pace.alpha,
				Rank:  rank,
				Ready: ready,
				Count: count,
			}

			if !yield(unsafe.Pointer(&pace.out)) {
				return
			}
		}
	}
}

func (pace *Pace) validate() error {
	for _, value := range []float64{
		pace.config.Rest, pace.config.Lower, pace.config.Upper,
		pace.config.Gain, pace.config.Band, pace.config.Window,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return core.ErrDomain
		}
	}

	if !(pace.config.Lower > 0) ||
		!(pace.config.Lower <= pace.config.Rest) ||
		!(pace.config.Rest <= pace.config.Upper) ||
		!(pace.config.Window > 0) ||
		math.Floor(pace.config.Window) != pace.config.Window {
		return core.ErrDomain
	}

	return nil
}
