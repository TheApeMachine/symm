package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestLogNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "log", NewLog(core.From(1.0)),
			tests.WithGenerator[float64](1, -5, 5, true),
		),
	).Run(t)
}
