package temporal

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Decay multiplies each arrival by a retention factor. The clock yields elapsed
time; the shape yields the factor for that elapsed time. A missing clock is
infinite elapsed time. A missing shape is linear retention, floored at zero.
*/
type Decay struct {
	err    error
	clock  core.Primitive
	shape  core.Primitive
	linear bool
	out    float64
}

func NewDecay(clock, shape core.Primitive) core.Primitive {
	return &Decay{
		clock:  clock,
		shape:  shape,
		linear: shape == nil,
	}
}

func (op *Decay) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			value := *(*float64)(arriving)
			elapsed := math.Inf(1)

			if op.clock != nil {
				clockIn := func(yieldClock func(unsafe.Pointer) bool) {
					yieldClock(arriving)
				}

				for tick := range op.clock.Next(clockIn) {
					elapsed = *(*float64)(tick)
				}
			}

			factor := elapsed

			if op.linear {
				factor = math.Max(0, 1-elapsed)
			} else if op.shape != nil {
				shapeIn := func(yieldShape func(unsafe.Pointer) bool) {
					yieldShape(unsafe.Pointer(&elapsed))
				}

				for out := range op.shape.Next(shapeIn) {
					factor = *(*float64)(out)
				}
			}

			op.out = value * factor

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Decay) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
