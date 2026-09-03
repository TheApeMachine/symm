package temporal

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Decay attenuates streaming signals over information or temporal horizons.
Degenerate zero-value behavior (Table 5.1):
- Rate omitted: instant drop to 0 (absence of clock implies elapsed time t -> infinity).
- Shape omitted: linear decay (absence of non-linear transfer function).
*/
type Decay struct {
	Rate  types.Node
	Shape types.Node

	last    types.Number
	hasLast bool
}

func (decay *Decay) Step(number types.Number) types.Number {
	// Degenerate rule: Rate omitted => instant drop to 0
	if decay.Rate == nil {
		return 0
	}

	elapsed := decay.Rate.Step(number)

	var factor types.Number

	if decay.Shape != nil {
		factor = decay.Shape.Step(elapsed)
	} else {
		// Default shape: Linear decay
		val := float64(elapsed)

		if val >= 1.0 {
			factor = 0
		} else if val <= 0 {
			factor = 1
		} else {
			factor = types.Number(1.0 - val)
		}
	}

	if !decay.hasLast {
		decay.last = number
		decay.hasLast = true

		return number
	}

	decay.last = decay.last*factor + number

	return decay.last
}
