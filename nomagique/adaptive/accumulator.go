package adaptive

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
accumulatorSlots names the running compensated-summation state an Accumulator
primitive keeps inside a frame.
*/
type accumulatorSlots struct {
	value types.Symbol // the signed sample this step integrates
	total types.Symbol // the accumulated level
	carry types.Symbol // the compensated-summation carry
	count types.Symbol // number of samples integrated
}

func newAccumulatorSlots(prefix string) accumulatorSlots {
	return accumulatorSlots{
		value: types.MustIntern(joinPrefix(prefix, "value")),
		total: types.MustIntern(joinPrefix(prefix, "total")),
		carry: types.MustIntern(joinPrefix(prefix, "carry")),
		count: types.MustIntern(joinPrefix(prefix, "count")),
	}
}

/*
Accumulator returns the primitive that integrates signed samples into a level
with compensated (Kahan) summation, so long streams retain low-order
contributions and reject non-finite overflow. The running total, carry, and
count live in the frame under the namespaced prefix.

It is a pure measurement: it adds each signed sample exactly once and reports
the running total. It never decides whether a level is high or low.
*/
func Accumulator(prefix string) types.Primitive {
	slots := newAccumulatorSlots(prefix)

	return func(input *types.Frame) {
		sample, found := input.Get(slots.value)

		if !found {
			input.Err = fmt.Errorf("adaptive: accumulator requires a value")

			return
		}

		if !utils.IsFinite(sample) {
			input.Err = fmt.Errorf("adaptive: accumulator value must be finite")

			return
		}

		total, hasTotal := input.Get(slots.total)
		carry, hasCarry := input.Get(slots.carry)
		count, hasCount := input.Get(slots.count)

		if !hasTotal {
			total = 0
		}

		if !hasCarry {
			carry = 0
		}

		if !hasCount {
			count = 0
		}

		compensated := sample - carry
		next := total + compensated
		nextCarry := next - total - compensated

		if !utils.IsFinite(next) {
			input.Err = fmt.Errorf("adaptive: accumulator sum overflowed to non-finite")

			return
		}

		total = next
		carry = nextCarry
		count++

		input.Put(slots.total, total)
		input.Put(slots.carry, carry)
		input.Put(slots.count, count)
	}
}
