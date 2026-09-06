package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestSubtractNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "subtract", Seed: 100, Operation: NewSubtract[float64](transport.NewIO(core.From(float64(100))))})
}
