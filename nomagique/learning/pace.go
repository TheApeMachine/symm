package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/calculus"
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
	mix        core.Primitive
	bound      core.Primitive
	adapter    *data.Adapter
	publish    data.Map[float64]
	calibrated data.Map[string]
	mixed      data.Map[string]
	bounded    data.Map[string]
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
		mix:            calculus.NewMix(),
		bound:          calculus.NewBound(),
		adapter:        data.NewAdapter(nil, data.NewState(data.NewMap())),
		publish:        data.NewOutputMap(),
		calibrated:     data.NewMap("value", "value", "prior_count", "prior_count"),
		mixed:          data.NewMap("mix", "mix"),
		bounded:        data.NewMap("bound", "bound"),
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

			op.publish.Values["value"] = val

			for range op.adapter.Next(data.NewValue(op.publish)) {
			}

			for range op.calibrator.Next(data.NewValue(op.adapter)) {
			}

			if err := op.calibrator.Error(); err != nil {
				op.Error(err)
				return
			}

			score, priorCount := 0.0, 0.0

			for pointer := range op.adapter.Next(data.NewValue(op.calibrated)) {
				values := (*data.Map[float64])(pointer).Values
				score = values["value"]
				priorCount = values["prior_count"]
			}

			if err := op.adapter.Error(); err != nil {
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

				op.publish.Values["left"] = op.logAlpha
				op.publish.Values["right"] = target
				op.publish.Values["weight"] = op.gain

				for range op.adapter.Next(data.NewValue(op.publish)) {
				}

				for range op.mix.Next(data.NewValue(op.adapter)) {
				}

				if err := op.mix.Error(); err != nil {
					op.Error(err)
					return
				}

				for pointer := range op.adapter.Next(data.NewValue(op.mixed)) {
					op.publish.Values["value"] = (*data.Map[float64])(pointer).Values["mix"]
				}

				for stage, limits := range [2][2]float64{{logMin, logMax}, {op.lower, op.upper}} {
					if stage == 1 {
						op.publish.Values["value"] = math.Exp(op.publish.Values["value"])
					}

					op.publish.Values["lower"] = limits[0]
					op.publish.Values["upper"] = limits[1]

					for range op.adapter.Next(data.NewValue(op.publish)) {
					}

					for range op.bound.Next(data.NewValue(op.adapter)) {
					}

					if err := op.bound.Error(); err != nil {
						op.Error(err)
						return
					}

					for pointer := range op.adapter.Next(data.NewValue(op.bounded)) {
						op.publish.Values["value"] = (*data.Map[float64])(pointer).Values["bound"]
					}

					if err := op.adapter.Error(); err != nil {
						op.Error(err)
						return
					}

					if stage == 0 {
						op.logAlpha = op.publish.Values["value"]
						continue
					}

					op.alpha = op.publish.Values["value"]
				}
			}

			readyFlag := 0.0

			if ready {
				readyFlag = 1
			}

			op.out = [4]float64{op.alpha, rank, readyFlag, count}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
