package hawkes_test

import (
	"github.com/theapemachine/symm/nomagique/equation/hawkes"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestLikelihoodNext(t *testing.T) {
	tests.CheckHawkes(t, transport.NewPipe(hawkes.NewLikelihood(), hawkes.NewGradient()))
}
