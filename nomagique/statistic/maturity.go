package statistic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
)

var SymbolMaturity = nomagique.MustIntern("maturity")

/*
Maturity maps an empirical support count to count/(count+1). The measure starts
at zero, rises monotonically, and approaches one without a chosen threshold.
*/
func Maturity(countSymbol nomagique.Symbol) nomagique.Primitive {
	return func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		count, found := input.Get(countSymbol)

		if !found || count < 0 {
			return state, nomagique.Frame{}, fmt.Errorf(
				"statistic: maturity requires a non-negative support count",
			)
		}

		output := input
		output.Put(SymbolMaturity, count/(count+1))

		return state, output, nil
	}
}
