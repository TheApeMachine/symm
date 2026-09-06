package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

/*
Erfc is not fuzzed: it maps the infinities onto finite values by definition,
so a poison cannot travel through it. That is the mathematics of the
transform rather than a value being quietly absorbed.
*/
func TestErfcNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "erfc", NewErfc(core.From(0.0)),
			tests.WithGenerator[float64](0, -3, 3, false),
		),
	).Run(t)
}
