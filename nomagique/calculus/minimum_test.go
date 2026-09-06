package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMinimumNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "minimum", NewMinimum(core.From(100.0)),
			tests.WithGenerator[float64](100, -10, 10, true),
		),
	).Run(t)
}
