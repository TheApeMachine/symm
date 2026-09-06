package adaptive_test

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestBaselineNext(t *testing.T) {
	tests.CheckCausalResidual(t, adaptive.NewBaseline(adaptive.NewWindow()), true)
}
