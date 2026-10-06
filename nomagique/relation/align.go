package relation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Align owns lagged as-of alignment of target observations against predictor
series. For a target observation at time t and a predictor with lag τ, the
aligned predictor is the newest observation at or before t - τ. Future
observations never enter a row. Only targets with every predictor aligned
are retained.

Each arrival is *[][]float64:

	[0]    predictor lags {τ1, ..., τN}
	[1]    target series {at0, raw0, at1, raw1, ...}
	[1+i]  predictor i series, flattened the same way

Every series must be chronological (resident windows are by construction);
one cursor per predictor scans each series once. It yields *[][]float64
rows in chronological target order, each {targetAt, targetRaw, p1At, p1Raw,
..., pNAt, pNRaw}. The rows reuse one buffer that the next arrival
overwrites.
*/
type Align struct {
	*core.PrimitiveError
	cursors []int
	flat    []float64
	out     [][]float64
}

/*
NewAlign creates an Align primitive.
*/
func NewAlign() *Align {
	return &Align{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Align) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			series := *(*[][]float64)(arriving)

			if len(series) < 2 || len(series) != 2+len(series[0]) {
				op.Error(core.ErrShape)
				return
			}

			lags, target := series[0], series[1]
			width := 2 + 2*len(lags)
			op.cursors = append(op.cursors[:0], make([]int, len(lags))...)

			for index := range op.cursors {
				op.cursors[index] = -1
			}

			op.flat = op.flat[:0]

			for targetIndex := 0; targetIndex+1 < len(target); targetIndex += 2 {
				at := target[targetIndex]
				start := len(op.flat)
				op.flat = append(op.flat, at, target[targetIndex+1])
				complete := len(lags) > 0

				for index, lag := range lags {
					history := series[2+index]
					cutoff := at - lag
					best := op.cursors[index]

					for cursor := max(best, 0); 2*cursor+1 < len(history) && history[2*cursor] <= cutoff; cursor++ {
						best = cursor
					}

					if best < 0 {
						complete = false
						break
					}

					op.cursors[index] = best
					op.flat = append(op.flat, history[2*best], history[2*best+1])
				}

				if !complete {
					op.flat = op.flat[:start]
				}
			}

			op.out = op.out[:0]

			for start := 0; start+width <= len(op.flat); start += width {
				op.out = append(op.out, op.flat[start:start+width])
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
