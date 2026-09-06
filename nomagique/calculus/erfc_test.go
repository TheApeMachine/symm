package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestErfcNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "erfc", Seed: 0, Operation: NewErfc(transport.NewIO(core.From(float64(0))))})
}
