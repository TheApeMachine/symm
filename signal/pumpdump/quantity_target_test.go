package pumpdump

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/transport"
)

func TestQuantityTargetNext(t *testing.T) {
	graph := newQuantityTarget()

	for _, sample := range [][2]float64{{2, 2}, {1, 1.5}, {.25, 1}, {.25, .625}, {1, 1}} {
		valueEval := transport.NewEvaluate(graph)
		var value float64

		for out := range valueEval.Next(transport.NewValues(sample[0]).Next(nil)) {
			value = *(*float64)(out)
		}

		err := valueEval.Error()

		if err != nil || value != sample[1] {
			t.Fatalf("quantity target: %g, want %g (%v)", value, sample[1], err)
		}
	}
}
