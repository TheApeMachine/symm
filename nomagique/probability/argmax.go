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
func NewArgmax() Argmax {
	return func(values []float64) ArgmaxResult {
		if len(values) == 0 {
			return ArgmaxResult{}
		}

		best := ArgmaxResult{Index: 0, Value: values[0]}
		for index := 1; index < len(values); index++ {
			if values[index] > best.Value {
				best = ArgmaxResult{Index: index, Value: values[index]}
			}
		}

		return best
	}
}
