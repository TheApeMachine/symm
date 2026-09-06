package learning_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestPriorNext(t *testing.T) {
	for _, memory := range []float64{0, 10} {
		tests.CheckPrior(t, learning.NewPrior(store.NewConstant(core.From(memory))), memory)
	}
}
