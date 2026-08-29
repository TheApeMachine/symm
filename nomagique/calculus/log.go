package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Log projects the natural logarithm of a finite positive X.
*/
func Log(input *types.Frame) {
	x, found := input.Get(PortX)

	if !found || !utils.IsFinite(x) || x <= 0 {
		input.Err = fmt.Errorf("calculus: log requires a finite positive x")

		return
	}

	input.Put(PortResult, math.Log(x))
}
