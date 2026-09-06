package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestMinimumNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "minimum", Seed: 100, Operation: NewMinimum(transport.NewIO(core.From(float64(100))))})
}
