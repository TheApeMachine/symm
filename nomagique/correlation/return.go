package correlation

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolReturn    = nomagique.MustIntern("return")
	SymbolMagnitude = nomagique.MustIntern("magnitude")
)

/*
Return emits the latest log return and its magnitude from a retained Path.
*/
func Return(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	count, _ := input.Get(nomagique.SampleCount)
	output := input

	if count < 2 {
		output.Put(SymbolReady, 0)

		return state, output, nil
	}

	_, previous, hasPrevious := temporal.PathSample(&input, int(count)-2)
	_, current, hasCurrent := temporal.PathSample(&input, int(count)-1)

	if !hasPrevious || !hasCurrent || previous <= 0 || current <= 0 {
		return state, types.Frame{}, fmt.Errorf(
			"correlation: return requires two positive path values",
		)
	}

	value := math.Log(current / previous)
	output.Put(SymbolReturn, value)
	output.Put(SymbolMagnitude, math.Abs(value))
	output.Put(SymbolReady, 1)

	return state, output, nil
}
