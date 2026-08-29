package hawkes

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
SymbolMark identifies the current event's aggressor side: +1 for an
aggressive buy, -1 for an aggressive sell.
*/
var SymbolMark = types.MustIntern("mark")

func finite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}
