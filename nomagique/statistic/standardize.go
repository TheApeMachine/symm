package statistic

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Standardize centers and scales streaming signals.
Degenerate zero-value behavior (Table 5.1):
- Center omitted: subtracts 0 (location defaults to zero).
- Scale omitted: divides by 1 (dispersion defaults to unity).
*/
type Standardize struct {
	Center types.Node
	Scale  types.Node
}

func (standardize *Standardize) Step(number types.Number) types.Number {
	center := types.Number(0)
	scale := types.Number(1)

	if standardize.Center != nil {
		center = standardize.Center.Step(number)
	}

	if standardize.Scale != nil {
		scale = standardize.Scale.Step(number)
	}

	if scale == 0 {
		return 0
	}

	return (number - center) / scale
}
