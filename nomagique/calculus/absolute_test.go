package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestAbsoluteNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "absolute", Seed: 0, Operation: NewAbsolute(transport.NewIO(core.From(float64(0))))})
}
