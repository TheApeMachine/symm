package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestAbsoluteNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "absolute", NewAbsolute(core.From(0.0)),
			tests.WithGenerator[float64](0, 0, 10, true),
		),
	).Run(t)
}
