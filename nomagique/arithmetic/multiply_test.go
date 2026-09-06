package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestMultiplyNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "multiply", Seed: 1, Operation: NewMultiply[float64](transport.NewIO(core.From(float64(1))))})
}
