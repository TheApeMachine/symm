package temporal

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

var (
	SymbolAge      = types.MustIntern("age")
	SymbolSpan     = types.MustIntern("span")
	SymbolProgress = types.MustIntern("progress")
)

/*
Clock calculates temporal progress as age divided by positive span.
*/
func Clock(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	age, hasAge := input.Get(SymbolAge)
	span, hasSpan := input.Get(SymbolSpan)

	if !hasAge || !hasSpan || !utils.IsFinite(age, span) {
		return state, types.Frame{}, fmt.Errorf(
			"temporal: clock requires finite age and span",
		)
	}

	if span <= 0 {
		return state, types.Frame{}, fmt.Errorf(
			"temporal: clock requires positive span",
		)
	}

	output := input
	output.Put(SymbolProgress, age/span)

	return state, output, nil
}
