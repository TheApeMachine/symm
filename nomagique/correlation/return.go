package correlation

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolReturn    = types.MustIntern("return")
	SymbolMagnitude = types.MustIntern("magnitude")
)

/*
Return emits the latest log return and its magnitude from a retained Path.
*/
func Return(input *types.Frame) {
	count, _ := input.Get(types.SampleCount)

	if count < 2 {
		input.Put(SymbolReady, 0)

		return
	}

	_, previous, hasPrevious := temporal.PathSample(input, int(count)-2)
	_, current, hasCurrent := temporal.PathSample(input, int(count)-1)

	if !hasPrevious || !hasCurrent || previous <= 0 || current <= 0 {
		input.Err = fmt.Errorf(
			"correlation: return requires two positive path values",
		)

		return
	}

	value := math.Log(current / previous)
	input.Put(SymbolReturn, value)
	input.Put(SymbolMagnitude, math.Abs(value))
	input.Put(SymbolReady, 1)
}
