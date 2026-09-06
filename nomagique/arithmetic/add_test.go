package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestAddNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "add", Seed: 0, Operation: NewAdd[float64](transport.NewIO(core.From(float64(0))))})
}
