package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Extent emits the absolute distance between A and B.
*/
func Extent(input *types.Frame) {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = fmt.Errorf("calculus: extent requires finite a and b")

		return
	}

	result := math.Abs(b - a)

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: extent overflowed")

		return
	}

	input.Put(PortResult, result)
}
