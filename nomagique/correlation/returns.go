package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Returns owns sequential log returns and cumulative energy of one price path.
Each arrival is *[][2]float64 prices as {at, value}; it yields *[2][]float64
where [0] is flattened returns {value, from, to, ...} and [1] is {energy}.
Non-positive prices report ErrDomain; non-advancing timestamps report ErrShape.
*/
type Returns struct {
	*core.PrimitiveError
	out   [2][]float64
	flat  []float64
	energy []float64
}

func NewReturns() core.Primitive {
	return &Returns{
		PrimitiveError: core.NewPrimitiveError(),
		energy:         make([]float64, 1),
	}
}

func (op *Returns) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			prices := *(*[][2]float64)(arriving)

			if len(prices) == 0 {
				op.flat = op.flat[:0]
				op.energy[0] = 0
				op.out[0] = op.flat
				op.out[1] = op.energy

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			if prices[0][1] <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			need := (len(prices) - 1) * 3

			if cap(op.flat) < need {
				op.flat = make([]float64, need)
			} else {
				op.flat = op.flat[:need]
			}

			prevLog := math.Log(prices[0][1])
			through := prices[0][0]
			energy := 0.0
			cursor := 0

			for index := 1; index < len(prices); index++ {
				at := prices[index][0]
				price := prices[index][1]

				if price <= 0 {
					op.Error(core.ErrDomain)
					return
				}

				if at <= through {
					op.Error(core.ErrShape)
					return
				}

				currLog := math.Log(price)
				retVal := currLog - prevLog
				energy += retVal * retVal
				op.flat[cursor] = retVal
				op.flat[cursor+1] = through
				op.flat[cursor+2] = at
				cursor += 3
				through = at
				prevLog = currLog
			}

			op.energy[0] = energy
			op.out[0] = op.flat
			op.out[1] = op.energy

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
