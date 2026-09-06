package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestExpNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "exp", Seed: 0, Operation: NewExp(transport.NewIO(core.From(float64(0))))})
}
