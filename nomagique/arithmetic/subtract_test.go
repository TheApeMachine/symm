package arithmetic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSubtractNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "subtract", NewSubtract(core.From(100.0)),
			tests.WithGenerator[float64](100, 1, 10, true),
		),
		tests.NewTestCase(
			"float64", "subtract", NewSubtract(core.From(100.0)),
			tests.WithGenerator[float64](100, -10, 0, true),
		),
	).Run(t)
}
