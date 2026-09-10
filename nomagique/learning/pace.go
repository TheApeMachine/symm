package learning

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
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
	core.Base[float64, PaceReading]
	config     PaceConfig
	calibrator *probability.Calibrator
	mix        *equation.Mix[float64]
	bound      *equation.Bound[float64]
	log        *calculus.Log[float64]
	exp        *calculus.Exp[float64]
	floor      *calculus.Floor[float64]
	finite     *logic.Finite[float64]
	logAlpha   float64
	alpha      float64
	seeded     bool
}

func NewPace(config PaceConfig) *Pace {
	return &Pace{
		config:     config,
		calibrator: probability.NewCalibrator(collection.NewTail[float64](int(config.Window))),
		mix:        equation.NewMix[float64](),
		bound:      equation.NewBound[float64](),
		log:        calculus.NewLog[float64](),
		exp:        calculus.NewExp[float64](),
		floor:      calculus.NewFloor[float64](),
		finite:     logic.NewFinite[float64](),
	}
}

func (op *Pace) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[PaceReading, PaceReading]] {
	return func(yield func(core.Primitive[PaceReading, PaceReading]) bool) {
		for arriving := range in {
			reading, err := op.Measure(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}

func (op *Pace) Measure(value float64) (PaceReading, error) {
	if err := op.validate(); err != nil {
		return PaceReading{}, err
	}

	defined, err := transport.Evaluate(op.finite, transport.Values(value))

	if err != nil {
		return PaceReading{}, err
	}

	if !defined {
		return PaceReading{}, core.ErrDomain
	}

	if !op.seeded {
		logRest, err := transport.Evaluate(op.log, transport.Values(op.config.Rest))

		if err != nil {
			return PaceReading{}, err
		}

		op.logAlpha = logRest
		op.alpha = op.config.Rest
		op.seeded = true
	}

	calibration, err := transport.Evaluate(op.calibrator, transport.Values(value))

	if err != nil {
		return PaceReading{}, err
	}

	ready := op.config.Window <= calibration.PriorCount
	count := math.Min(op.config.Window, calibration.PriorCount+1)
	rank := 0.0

	if ready {
		rank = calibration.Value
		logRest, err := transport.Evaluate(op.log, transport.Values(op.config.Rest))

		if err != nil {
			return PaceReading{}, err
		}

		logMin, err := transport.Evaluate(op.log, transport.Values(op.config.Lower))

		if err != nil {
			return PaceReading{}, err
		}

		logMax, err := transport.Evaluate(op.log, transport.Values(op.config.Upper))

		if err != nil {
			return PaceReading{}, err
		}

		target := logRest

		if rank < op.config.Band {
			target = logMax
		}

		if rank > 1-op.config.Band {
			target = logMin
		}

		mixed, err := transport.Evaluate(op.mix, transport.Values(equation.MixRecord[float64]{
			Left:   op.logAlpha,
			Right:  target,
			Weight: op.config.Gain,
		}))

		if err != nil {
			return PaceReading{}, err
		}

		logAlpha, err := transport.Evaluate(op.bound, transport.Values(equation.BoundRecord[float64]{
			Value: mixed,
			Lower: logMin,
			Upper: logMax,
		}))

		if err != nil {
			return PaceReading{}, err
		}

		alpha, err := transport.Evaluate(op.exp, transport.Values(logAlpha))

		if err != nil {
			return PaceReading{}, err
		}

		alpha, err = transport.Evaluate(op.bound, transport.Values(equation.BoundRecord[float64]{
			Value: alpha,
			Lower: op.config.Lower,
			Upper: op.config.Upper,
		}))

		if err != nil {
			return PaceReading{}, err
		}

		op.logAlpha = logAlpha
		op.alpha = alpha
	}

	return PaceReading{
		Alpha: op.alpha,
		Rank:  rank,
		Ready: ready,
		Count: count,
	}, nil
}

func (op *Pace) validate() error {
	for _, value := range []float64{
		op.config.Rest, op.config.Lower, op.config.Upper,
		op.config.Gain, op.config.Band, op.config.Window,
	} {
		defined, err := transport.Evaluate(op.finite, transport.Values(value))

		if err != nil {
			return err
		}

		if !defined {
			return core.ErrDomain
		}
	}

	floored, err := transport.Evaluate(op.floor, transport.Values(op.config.Window))

	if err != nil {
		return err
	}

	if !(op.config.Lower > 0) ||
		!(op.config.Lower <= op.config.Rest) ||
		!(op.config.Rest <= op.config.Upper) ||
		!(op.config.Window > 0) ||
		floored != op.config.Window {
		return core.ErrDomain
	}

	return nil
}
