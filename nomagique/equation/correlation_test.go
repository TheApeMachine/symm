package equation_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func TestNewCorrelation(t *testing.T) {
	node := equation.NewCorrelation()
	for _, covariance := range []float64{2, -2, 0, 1} {
		out := tests.Drain(t, node, tests.Values(tests.Record(map[string]any{"covariance": covariance, "left_energy": 1.0, "right_energy": 2.0})))
		tests.Sound(t, node)
		if len(out) != 1 {
			t.Fatal("expected one correlation")
		}
		tests.EqualNumber(t, out[0], covariance/math.Sqrt2)
	}
	out := tests.Drain(t, node, tests.Values(tests.Record(map[string]any{"covariance": 0.0, "left_energy": 0.0, "right_energy": 0.0})))
	tests.Sound(t, node)
	if len(out) != 1 {
		t.Fatal("expected undefined numeric result")
	}
	tests.EqualNumber(t, out[0], math.NaN())
}

func BenchmarkNewCorrelation(b *testing.B) {
	correlation := equation.NewCorrelation()
	input := transport.NewIO(tests.Record(map[string]any{
		"covariance": 2.0, "left_energy": 1.0, "right_energy": 2.0,
	}))
	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		output := correlation.Next(input)

		if output == nil || core.To[float64](output) != 2/math.Sqrt(2) {
			b.Fatal("incorrect correlation output")
		}

		if correlation.Next(input) != nil {
			b.Fatal("correlation delivery did not end")
		}
	}

	if err := correlation.Error(); err != nil {
		b.Fatal(err)
	}
}
