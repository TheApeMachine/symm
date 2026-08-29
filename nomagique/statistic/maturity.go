package statistic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
)

var SymbolMaturity = types.MustIntern("maturity")

/*
Maturity maps an empirical support count to count/(count+1). The measure starts
at zero, rises monotonically, and approaches one without a chosen threshold.
*/
func Maturity(countSymbol types.Symbol) types.Primitive {
	return func(input *types.Frame) {
		count, found := input.Get(countSymbol)

		if !found || count < 0 {
			input.Err = fmt.Errorf(
				"statistic: maturity requires a non-negative support count",
			)

			return
		}

		input.Put(SymbolMaturity, count/(count+1))
	}
}
