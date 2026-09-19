package probability

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
ArgmaxResult is a winning value and the first index at which it occurred.
*/
type ArgmaxResult struct {
	Index int
	Value float64
}

/*
NewArgmax preserves a winning value's ordinal through comparison.
No structs, pure Value closure.
*/
type Argmax types.Value[[]float64, ArgmaxResult]

func NewArgmax(values ...types.Float) Argmax {
	return func(in []float64) ArgmaxResult {
		vals := in
		if len(values) > 0 {
			vals = make([]float64, len(values))
			for i, v := range values {
				if v != nil {
					vals[i] = v(in)
				}
			}
		}

		if len(vals) == 0 {
			return ArgmaxResult{}
		}

		best := ArgmaxResult{Index: 0, Value: vals[0]}
		for index := 1; index < len(vals); index++ {
			if vals[index] > best.Value {
				best = ArgmaxResult{Index: index, Value: vals[index]}
			}
		}

		return best
	}
}
