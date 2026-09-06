package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

/*
Exp is not fuzzed: a negative infinity maps onto zero by definition, so a
poison cannot travel through it. That is the mathematics of the transform
rather than a value being quietly absorbed.
*/
func TestExpNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "exp", NewExp(core.From(0.0)),
			tests.WithGenerator[float64](0, 0, 5, false),
		),
	).Run(t)
}
