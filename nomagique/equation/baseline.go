package equation

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
CausalBaseline is a structural composition (Tier 4 Equation).
It embeds the adaptive.Baseline primitive by value, executing with zero
custom arithmetic, zero allocations, and zero magic constants.
*/
type CausalBaseline struct {
	baseline adaptive.Baseline
}

func (equation *CausalBaseline) Step(number nmtypes.Number) nmtypes.Number {
	return equation.baseline.Step(number)
}

func (equation *CausalBaseline) Baseline() float64 {
	return equation.baseline.Engine.Mean()
}

func (equation *CausalBaseline) Mean() float64 {
	return equation.baseline.Engine.Mean()
}

func (equation *CausalBaseline) Dispersion() float64 {
	return equation.baseline.Engine.Dispersion()
}

func (equation *CausalBaseline) Count() float64 {
	return equation.baseline.Engine.Count()
}
