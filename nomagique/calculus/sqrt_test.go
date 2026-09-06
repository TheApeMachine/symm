package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSqrtNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "sqrt", NewSqrt(core.From(0.0)),
			tests.WithGenerator[float64](0, -5, 5, true),
		),
	).Run(t)
}
