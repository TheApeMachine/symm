package temporal

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
)

var (
	SymbolAge      = nomagique.MustIntern("age")
	SymbolSpan     = nomagique.MustIntern("span")
	SymbolProgress = nomagique.MustIntern("progress")
)

/*
Clock calculates temporal progress as age divided by positive span.
*/
func Clock(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	age, hasAge := input.Get(SymbolAge)
	span, hasSpan := input.Get(SymbolSpan)

	if !hasAge || !hasSpan || !finite(age, span) {
		return state, nomagique.Frame{}, fmt.Errorf(
			"temporal: clock requires finite age and span",
		)
	}

	if span <= 0 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"temporal: clock requires positive span",
		)
	}

	output := input
	output.Put(SymbolProgress, age/span)

	return state, output, nil
}

func finite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}
