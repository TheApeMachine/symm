package equation_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestPolarizeNext(t *testing.T) {
	node := equation.NewPolarize(store.NewConstant(core.From(10.0)))
	results := tests.Drain(t, node, tests.Values(10.0, -10.0, 0.0))
	tests.Sound(t, node)
	for index, expected := range []float64{0.5, -0.5, 0} {
		tests.EqualNumber(t, tests.Number(t, tests.Fields(t, results[index]), "value"), expected)
	}
}
