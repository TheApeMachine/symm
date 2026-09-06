package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestSquareNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "square", Seed: 0, Operation: NewSquare(transport.NewIO(core.From(float64(0))))})
}
