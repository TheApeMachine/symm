package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestReciprocalNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "reciprocal", Seed: 1, Operation: NewReciprocal(transport.NewIO(core.From(float64(1))))})
}
