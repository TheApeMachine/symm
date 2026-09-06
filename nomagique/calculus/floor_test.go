package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestFloorNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "floor", Seed: 0, Operation: NewFloor(transport.NewIO(core.From(0.0)))})
}
