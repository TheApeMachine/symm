package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

/*
Reciprocal is not fuzzed: an infinity has zero for its inverse, so a poison
cannot travel through it. That is the mathematics of the transform rather
than a value being quietly absorbed.
*/
func TestReciprocalNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "reciprocal", NewReciprocal(core.From(1.0)),
			tests.WithGenerator[float64](1, -5, 5, false),
		),
	).Run(t)
}
