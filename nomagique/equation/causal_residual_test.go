package equation_test

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestCausalResidualNext(t *testing.T) {
	tests.CheckCausalResidual(t, equation.NewCausalResidual(algo.NewWelford()), false)
}
