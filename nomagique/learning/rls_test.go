package learning_test

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func TestRLSNext(t *testing.T) {
	for _, fixture := range []struct {
		dimension        int
		variance, lambda float64
	}{{1, 100, 1}, {3, 10, 0.98}, {4, 1, 0.995}, {154, 1, 1}} {
		Convey(fmt.Sprintf("RLS dimension %d preserves prior predictions and posterior covariance", fixture.dimension), t, func() {
			state := store.NewRetained(nil)
			node := learning.NewRLS(store.NewConstant(core.From(float64(fixture.dimension))),
				store.NewConstant(core.From(fixture.variance)), store.NewConstant(core.From(fixture.lambda)))
			tests.CheckRLS(t, transport.NewPipe(node, state), fixture.dimension,
				fixture.variance, fixture.lambda, learning.NewRLSSum(state))
		})
	}
}

// BenchmarkRLSNext uses the 154-feature readout observed in the live resonance
// profile. Each iteration trains one observation and then queries that posterior.
func BenchmarkRLSNext(b *testing.B) {
	const dimension = 154
	node := learning.NewRLS(store.NewConstant(core.From(float64(dimension))),
		store.NewConstant(core.From(1.0)), store.NewConstant(core.From(1.0)))
	features := make([]float64, dimension)
	query := core.Record(map[string]any{"features": features})
	step := 0
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for index := range features {
			features[index] = math.Sin(float64((step + 1) * (index + 1)))
		}
		observation := core.Record(map[string]any{"features": features, "target": features[0] - features[1]})

		if _, err := transport.Evaluate[map[string]core.Primitive](node, observation); err != nil {
			b.Fatal(err)
		}

		if _, err := transport.Evaluate[map[string]core.Primitive](node, query); err != nil {
			b.Fatal(err)
		}
		step++
	}
}
