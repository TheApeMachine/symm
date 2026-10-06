package statistic

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
PriorEstimator owns the reliability-weighted prior recurrence as a Primitive:
normalized reliability-weighted moments and their causal clock.

Each arrival is *[6]float64 {value, authority, memory, epoch, has epoch,
age only}. A completion carries a value with its authority in [0, 1] under a
memory; an age-only query (age only = 1, with an epoch when has epoch = 1)
discounts evidence without counting a sample.

It yields one *[11]float64 summary separating completion, support, dispersion
and retained authority:

	[0] samples   [1] pending   [2] defined (1 or 0)  [3] variance defined (1 or 0)
	[4] mean      [5] variance  [6] support           [7] maturity
	[8] evidence authority      [9] authority         [10] memory
*/
type PriorEstimator struct {
	*core.PrimitiveError
	samples   float64
	pending   float64
	lastEpoch float64
	mean      float64
	weight    float64
	support   float64
	moment    float64
	out       [11]float64
}

/*
NewPriorEstimator instantiates the reliability-weighted prior Primitive.
*/
func NewPriorEstimator() *PriorEstimator {
	return &PriorEstimator{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Next applies one observation or age-only query to the prior recurrence and
hands over the resulting summary.
*/
func (op *PriorEstimator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			observation := (*[6]float64)(arriving)
			value, authority, memory := observation[0], observation[1], observation[2]
			epoch, hasEpoch, ageOnly := observation[3], observation[4] != 0, observation[5] != 0

			if !ageOnly && !(authority >= 0 && authority <= 1) {
				op.Error(fmt.Errorf("%w: prior authority must be in [0, 1]", core.ErrDomain))
				return
			}

			if !ageOnly {
				op.samples++
			}

			// Age discounts total weight; normalized moment and support are
			// scale invariant. A completion ages only through positive authority.
			aging := hasEpoch && (ageOnly || authority != 0)

			if aging && epoch > op.lastEpoch {
				if memory > 1 {
					gap := epoch - op.lastEpoch
					op.weight *= math.Exp(gap * math.Log(1-1/memory))
				}

				op.lastEpoch = epoch
			}

			completion := !ageOnly && authority != 0

			if completion && !hasEpoch && memory > 1 {
				op.weight *= math.Exp(math.Log(1 - 1/memory))
			}

			if completion && op.weight == 0 {
				op.mean, op.weight = value, authority
				op.support, op.moment = 1, 0
				completion = false
			}

			if completion {
				total := op.weight + authority
				retained, incoming := op.weight/total, authority/total
				difference := value - op.mean
				op.support = 1 / (retained*retained/op.support + incoming*incoming)
				op.moment = retained*op.moment + (retained*incoming)*(difference*difference)
				op.mean += incoming * difference
				op.weight = total
			}

			op.out = [11]float64{op.samples, op.pending}
			op.out[10] = memory

			if op.weight > 0 {
				op.out[2] = 1
				op.out[4] = op.mean
				op.out[6] = op.support
				op.out[8] = op.weight / op.support
			}

			if op.weight > 0 && op.support > 1 {
				op.out[3] = 1
				op.out[5] = op.moment * (op.support / (op.support - 1))
				op.out[7] = (op.support - 1) / op.support
				power := op.mean * op.mean
				totalPower := power + op.out[5]

				if totalPower > 0 {
					op.out[9] = (op.out[7] * op.out[8]) * (power / totalPower)
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
