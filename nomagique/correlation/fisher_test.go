package correlation_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
	"math"
	"testing"
)

func TestFisherNext(t *testing.T) {
	node := correlation.NewFisher()
	for _, c := range []struct {
		r, n    float64
		defined bool
	}{
		{0, 103, true}, {.8, 103, true}, {-.8, 103, true}, {1, 103, true}, {-1, 103, true},
		{1.2, 103, false}, {.8, 3, false}, {.8, 0, false}, {math.NaN(), 103, false}, {.5, 103, true},
	} {
		out := tests.Drain(t, node, tests.Values(tests.Record(map[string]any{
			"correlation": c.r, "support": c.n, "search_count": 20.0})))
		tests.Sound(t, node)
		f := tests.Fields(t, out[0])
		if core.To[bool](f["defined"]) != c.defined {
			t.Fatal(c)
		}
		expected := math.NaN()
		adjusted := math.NaN()
		if c.defined {
			expected = math.Erfc(math.Abs(math.Atanh(c.r)*math.Sqrt(c.n-3)) / math.Sqrt2)
			adjusted = math.Min(1, 20*expected)
		}
		tests.EqualNumber(t, tests.Number(t, f, "p_value"), expected)
		tests.EqualNumber(t, tests.Number(t, f, "search_adjusted_p_value"), adjusted)
	}
}
