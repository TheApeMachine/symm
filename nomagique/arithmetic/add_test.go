package arithmetic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestAddNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "add", NewAdd(core.From(0.0)),
			tests.WithGenerator[float64](0, 0, 10, true),
		),
		tests.NewTestCase(
			"float64", "add", NewAdd(core.From(0.0)),
			tests.WithGenerator[float64](0, -10, 0, true),
		),
	).Run(t)
}
