package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
)

/*
Observe requires an input to carry each configured coordinate. It validates
shape only; numeric failures are deliberately left visible to the primitive
that owns the corresponding mathematical domain.
*/
func Observe(required ...nomagique.Symbol) nomagique.Primitive {
	return func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		for _, symbol := range required {
			if input.Has(symbol) {
				continue
			}

			name, found := nomagique.SymbolName(symbol)

			if !found {
				return state, nomagique.Frame{}, fmt.Errorf(
					"logic: observation is missing coordinate %d",
					symbol,
				)
			}

			return state, nomagique.Frame{}, fmt.Errorf(
				"logic: observation is missing coordinate %s",
				name,
			)
		}

		return state, input, nil
	}
}
