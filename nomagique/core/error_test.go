package core_test

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Type failures must survive both the returned value and the run boundary.
func TestErrorTravels(t *testing.T) {
	tests.CheckTypeFailure(t, arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))))
}
