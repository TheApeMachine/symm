package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
LinearNode is a pure linear transfer function max(0, 1 - x).
*/
type LinearNode struct{}

func (linear LinearNode) Step(number types.Number) types.Number {
	val := float64(number)

	if val >= 1.0 {
		return 0
	}

	if val <= 0 {
		return 1
	}

	return types.Number(1.0 - val)
}
