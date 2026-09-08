package prior_test

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/prior"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestPrimitiveNext(t *testing.T) {
	for _, memory := range []float64{0, 10} {
		tests.CheckPrior(t, prior.New(store.NewConstant(core.From(memory)), prior.NewMemory()), memory)
	}
}
