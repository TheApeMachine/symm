package pumpdump

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/transport"
)

func TestQuantityTargetNext(t *testing.T) {
	graph := newQuantityTarget()

	for _, sample := range [][2]float64{{2, 2}, {1, 1.5}, {.25, 1}, {.25, .625}, {1, 1}} {
		value, err := transport.Evaluate(graph, transport.Values(sample[0]))

		if err != nil || value != sample[1] {
			t.Fatalf("quantity target: %g, want %g (%v)", value, sample[1], err)
		}
	}
}
