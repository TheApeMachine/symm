package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Atanh projects the inverse hyperbolic tangent of a finite X with |X| < 1, the
Fisher transform of a correlation coefficient.
*/
func Atanh(input *types.Frame) {
	x, found := input.Get(PortX)

	if !found || !utils.IsFinite(x) || x <= -1 || x >= 1 {
		input.Err = fmt.Errorf("calculus: atanh requires a finite x strictly between -1 and 1")

		return
	}

	input.Put(PortResult, math.Atanh(x))
}
