package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestAtanhNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "atanh", NewAtanh(core.From(0.0)),
			tests.WithGenerator[float64](0, -3, 3, true),
		),
	).Run(t)
}
