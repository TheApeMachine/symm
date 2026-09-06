package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
	"math/rand"
	"sort"
	"testing"
)

// CheckHawkes compares a pairwise composition to the original chronological
// grouped-event likelihood/score. Simultaneous events, prehistory and events
// beyond the observation horizon are included in deterministic fixtures.
func CheckHawkes(t *testing.T, node core.Primitive) {
	t.Helper()
	random := rand.New(rand.NewSource(934))
	for trial := 0; trial < 12; trial++ {
		marked := []referenceHawkesMarkedEvent{{-1, 0}, {0, 1}, {0, 0}, {1, 1}, {1, 0}, {10, 1}}
		for i := 0; i < 8+trial; i++ {
			marked = append(marked, referenceHawkesMarkedEvent{random.Float64() * 7, random.Intn(2)})
		}
		sort.SliceStable(marked, func(i, j int) bool { return marked[i].atSec < marked[j].atSec })
		stream := referenceHawkesArrivalStream{marked: marked, originSec: 0}
		events := []core.Primitive{}
		for _, e := range marked {
			events = append(events, Record(map[string]any{"at": e.atSec, "side": float64(e.side)}))
			if e.side == 0 {
				stream.buy = append(stream.buy, e.atSec)
			} else {
				stream.sell = append(stream.sell, e.atSec)
			}
		}
		params := []float64{0.3 + random.Float64(), 0.3 + random.Float64(), 0.12, 0.07, 0.06, 0.1, 0.7 + random.Float64()}
		fit := referenceHawkesBivariateFit{params[0], params[1], params[2], params[3], params[4], params[5], params[6]}
		expected, gradient, ok := fit.logLikelihoodGradient(stream, 6.5)
		if !ok {
			t.Fatal("reference rejected fixture")
		}
		input := map[string]any{"events": events, "origin": 0.0, "horizon": 6.5}
		names := []string{"mu_x", "mu_y", "alpha_xx", "alpha_xy", "alpha_yx", "alpha_yy", "beta"}
		for i, n := range names {
			input[n] = params[i]
		}
		out := Drain(t, node, Values(Record(input)))
		Sound(t, node)
		f := Fields(t, out[0])
		EqualNumber(t, Number(t, f, "log_likelihood"), expected)
		actual := core.To[[]float64](f["gradient"])
		for i, value := range gradient {
			EqualNumber(t, actual[i], value)
		}
		if trial == 0 {
			const step = 1e-6
			for i, name := range names {
				input[name] = params[i] + step
				upper := Drain(t, node, Values(Record(input)))
				Sound(t, node)
				hi := Number(t, Fields(t, upper[0]), "log_likelihood")
				input[name] = params[i] - step
				lower := Drain(t, node, Values(Record(input)))
				Sound(t, node)
				lo := Number(t, Fields(t, lower[0]), "log_likelihood")
				input[name] = params[i]
				numeric := (hi - lo) / (2 * step)
				if math.Abs(numeric-actual[i]) > 1e-5*(1+math.Abs(numeric)) {
					t.Fatalf("gradient %s: %g vs numeric %g", name, actual[i], numeric)
				}
			}
		}
	}
}
