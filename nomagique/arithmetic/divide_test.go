package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestDivideNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "divide", Seed: 1024, Operation: NewDivide[float64](transport.NewIO(core.From(float64(1024))))})
}
