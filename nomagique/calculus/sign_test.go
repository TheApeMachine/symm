package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

/*
Sign is not fuzzed: it emits only minus one, zero, or one, so no input of any
kind travels through it as itself. Discarding magnitude is the whole of what
the transform does.
*/
func TestSignNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "sign", NewSign(core.From(0.0)),
			tests.WithGenerator[float64](0, -5, 5, false),
		),
	).Run(t)
}
