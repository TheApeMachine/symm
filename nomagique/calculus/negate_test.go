package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestNegateNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "negate", Seed: 0, Operation: NewNegate(transport.NewIO(core.From(float64(0))))})
}
