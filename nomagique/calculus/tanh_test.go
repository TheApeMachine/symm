package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestTanhNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "tanh", Seed: 0, Operation: NewTanh(transport.NewIO(core.From(float64(0))))})
}
