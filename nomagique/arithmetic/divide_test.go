package arithmetic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestDivideNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "divide", NewDivide(core.From(1024.0)),
			tests.WithGenerator[float64](1024, 1, 10, true),
		),
		tests.NewTestCase(
			"float64", "divide", NewDivide(core.From(1024.0)),
			tests.WithGenerator[float64](1024, -10, 0, true),
		),
	).Run(t)
}
