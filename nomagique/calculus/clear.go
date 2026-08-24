package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Clear removes configured coordinates from committed state.
*/
func Clear(symbols ...types.Symbol) types.Primitive {
	configured := append([]types.Symbol(nil), symbols...)

	return func(input types.Frame) types.Frame {
		for _, symbol := range configured {
			input.Delete(symbol)
		}

		return input
	}
}
