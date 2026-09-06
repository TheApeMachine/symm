package arithmetic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMultiplyNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "multiply", NewMultiply(core.From(1.0)),
			tests.WithGenerator[float64](1, 1, 10, true),
		),
		tests.NewTestCase(
			"float64", "multiply", NewMultiply(core.From(1.0)),
			tests.WithGenerator[float64](1, -10, 0, true),
		),
	).Run(t)
}
