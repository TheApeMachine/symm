package learning_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestRLSNext(t *testing.T) {
	for _, c := range []struct {
		dimension        int
		variance, lambda float64
	}{{1, 100, 1}, {3, 10, 0.98}, {4, 1, 0.995}} {
		state := store.NewRetained(nil)
		tests.CheckRLS(t, transport.NewPipe(learning.NewRLS(store.NewConstant(core.From(float64(c.dimension))), store.NewConstant(core.From(c.variance)), store.NewConstant(core.From(c.lambda))), state), c.dimension, c.variance, c.lambda, learning.NewRLSSum(state))
	}
}
