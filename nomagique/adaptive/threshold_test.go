package adaptive_test

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestThresholdNext(t *testing.T) {
	tests.CheckThreshold(t, adaptive.NewThreshold(algo.NewWelford(), equation.NewPredictiveInflation()), "predictive")
	tests.CheckThreshold(t, adaptive.NewThreshold(algo.NewWelford(), store.NewConstant(core.From(1.482602218505602))), "normal")
	tests.CheckThreshold(t, adaptive.NewThreshold(algo.NewWelford(), transport.NewPipe(store.NewGet("count"), calculus.NewSqrt(transport.NewIO(core.From(0.0))))), "chebyshev")
}
