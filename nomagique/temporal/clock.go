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
func Clock(input *types.Frame) {
	age, hasAge := input.Get(SymbolAge)
	span, hasSpan := input.Get(SymbolSpan)

	if !hasAge || !hasSpan || !utils.IsFinite(age, span) {
		input.Err = fmt.Errorf(
			"temporal: clock requires finite age and span",
		)

		return
	}

	if span <= 0 {
		input.Err = fmt.Errorf(
			"temporal: clock requires positive span",
		)

		return
	}

	input.Put(SymbolProgress, age/span)
}
