package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestSqrtNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "sqrt", Seed: 0, Operation: NewSqrt(transport.NewIO(core.From(float64(0))))})
}
