package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestAtanhNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "atanh", Seed: 0, Operation: NewAtanh(transport.NewIO(core.From(float64(0))))})
}
