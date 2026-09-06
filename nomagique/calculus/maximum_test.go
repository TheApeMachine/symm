package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestMaximumNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "maximum", Seed: -100, Operation: NewMaximum(transport.NewIO(core.From(float64(-100))))})
}
