package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/probability"
)

/*
Pace owns empirical prior-error rank and the bounded log-alpha controller.
The calibrator owns history; Mix owns movement; Bound owns the limits.

Rest, bounds, gain, band, and window are explicit configuration, not newly
chosen tuning constants. A rejected configuration is recorded at construction
and every stream over it yields nothing.

Each arrival is *float64 (the current error); it yields *[4]float64
{alpha, rank, ready, count}.
*/
type Pace struct {
	*core.PrimitiveError
	rest       float64
	lower      float64
	upper      float64
	gain       float64
	band       float64
	window     float64
	calibrator core.Primitive
	logAlpha   float64
	alpha      float64
	seeded     bool
	out        [4]float64
}

func NewPace(rest, lower, upper, gain, band, window float64) core.Primitive {
	pace := &Pace{
		PrimitiveError: core.NewPrimitiveError(),
		rest:           rest,
		lower:          lower,
		upper:          upper,
		gain:           gain,
		band:           band,
		window:         window,
	}

	for _, value := range []float64{rest, lower, upper, gain, band, window} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			pace.Error(core.ErrDomain)
			return pace
		}
	}

	if !(lower > 0) || !(lower <= rest) || !(rest <= upper) ||
		!(window > 0) || math.Floor(window) != window {
		pace.Error(core.ErrDomain)
		return pace
	}

	pace.calibrator = probability.NewCalibrator(collection.NewTail[float64](int(window)))
	return pace
}

func (op *Pace) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.Error() != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)

			if math.IsNaN(val) || math.IsInf(val, 0) {
				op.Error(core.ErrDomain)
				return
			}

			if !op.seeded {
				op.logAlpha = math.Log(op.rest)
				op.alpha = op.rest
				op.seeded = true
			}

			score := 0.0
			priorCount := 0.0

			for out := range op.calibrator.Next(data.NewValue(val).Next(nil)) {
				reading := (*probability.CalibratorReading)(out)
				score = reading.Value
				priorCount = reading.PriorCount
			}

			if err := op.calibrator.Error(); err != nil {
				op.Error(err)
				return
			}

			ready := op.window <= priorCount
			count := math.Min(op.window, priorCount+1)
			rank := 0.0

			if ready {
				rank = score
				logMin := math.Log(op.lower)
				logMax := math.Log(op.upper)
				target := math.Log(op.rest)

				if rank < op.band {
					target = logMax
				}

				if rank > 1-op.band {
					target = logMin
				}

				op.logAlpha = (1-op.gain)*op.logAlpha + op.gain*target
				if op.logAlpha < logMin {
					op.logAlpha = logMin
				}

				if op.logAlpha > logMax {
					op.logAlpha = logMax
				}

				op.alpha = math.Exp(op.logAlpha)
				if op.alpha < op.lower {
					op.alpha = op.lower
				}

				if op.alpha > op.upper {
					op.alpha = op.upper
				}
			}

			readyFlag := 0.0
			if ready {
				readyFlag = 1.0
			}

			op.out = [4]float64{op.alpha, rank, readyFlag, count}

			for value := range data.NewValue(op.out).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
