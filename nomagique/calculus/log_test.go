package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestLogNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "log", Seed: 1, Operation: NewLog(transport.NewIO(core.From(float64(1))))})
}
