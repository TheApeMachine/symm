package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
PositiveOrder validates two configured facts as 0 < lower < upper.
*/
func PositiveOrder(lower types.Symbol, upper types.Symbol) types.Primitive {
	return func(input types.Frame) types.Frame {
		lowerValue, hasLower := input.Get(lower)
		upperValue, hasUpper := input.Get(upper)

		if !hasLower || !hasUpper || !utils.IsFinite(lowerValue) ||
			!utils.IsFinite(upperValue) || lowerValue <= 0 || upperValue <= lowerValue {
			input.Err = fmt.Errorf("logic: positive order requires 0 < lower < upper")

			return input
		}

		return input
	}
}
