package temporal

import (
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
	*core.PrimitiveError

	clock  core.Primitive
	shape  core.Primitive
	linear bool
	out    float64
}

func NewDecay(clock, shape core.Primitive) *Decay {
	return &Decay{PrimitiveError: core.NewPrimitiveError(), clock: clock,
		shape:  shape,
		linear: shape == nil,
	}
}

func (decay *Decay) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			value := *(*float64)(arriving)
			elapsed := math.Inf(1)

			if decay.clock != nil {
				clockIn := func(yieldClock func(unsafe.Pointer) bool) {
					yieldClock(arriving)
				}

				for tick := range decay.clock.Next(clockIn) {
					elapsed = *(*float64)(tick)
				}
			}

			factor := elapsed

			if decay.linear {
				factor = math.Max(0, 1-elapsed)
			} else if decay.shape != nil {
				shapeIn := func(yieldShape func(unsafe.Pointer) bool) {
					yieldShape(unsafe.Pointer(&elapsed))
				}

				for out := range decay.shape.Next(shapeIn) {
					factor = *(*float64)(out)
				}
			}

			decay.out = value * factor

			if !yield(unsafe.Pointer(&decay.out)) {
				return
			}
		}
	}
}
