package learning_test

import (
	"fmt"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestRLSNext(t *testing.T) {
	for _, fixture := range []struct {
		dimension        int
		variance, lambda float64
	}{{1, 100, 1}, {3, 10, 0.98}, {4, 1, 0.995}} {
		Convey(fmt.Sprintf("RLS dimension %d predicts before it trains", fixture.dimension), t, func() {
			node := learning.NewRLS(fixture.dimension, fixture.variance, fixture.lambda)
			features := make([]float64, fixture.dimension)

			for index := range features {
				features[index] = float64(index + 1)
			}

			firstEval := transport.NewEvaluate(node)
			var first algo.Reading

			for out := range firstEval.Next(transport.NewValues(learning.Sample{
				Features: features,
				Target:   1,
				Observed: true,
			}).Next(nil)) {
				first = *(*algo.Reading)(out)
			}

			err := firstEval.Error()
			_ = first
			So(err, ShouldBeNil)

			queryEval := transport.NewEvaluate(node)
			var query algo.Reading

			for out := range queryEval.Next(transport.NewValues(learning.Sample{Features: features}).Next(nil)) {
				query = *(*algo.Reading)(out)
			}

			err = queryEval.Error()
			So(err, ShouldBeNil)
			So(query.Observed, ShouldBeFalse)
			So(query.Beta, ShouldResemble, first.Beta)
			prediction := first.Beta[0]

			for index, feature := range features {
				prediction += first.Beta[index+1] * feature
			}

			So(query.Prediction, ShouldAlmostEqual, prediction)
		})
	}
}

func BenchmarkRLSNext(b *testing.B) {
	const dimension = 154
	node := learning.NewRLS(dimension, 1, 1)
	features := make([]float64, dimension)
	step := 0
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for index := range features {
			features[index] = math.Sin(float64((step + 1) * (index + 1)))
		}

		trained := transport.NewEvaluate(node)

		for out := range trained.Next(transport.NewValues(learning.Sample{
			Features: features,
			Target:   features[0] - features[1],
			Observed: true,
		}).Next(nil)) {
			_ = *(*algo.Reading)(out)
		}

		if err := trained.Error(); err != nil {
			b.Fatal(err)
		}

		predicted := transport.NewEvaluate(node)

		for out := range predicted.Next(transport.NewValues(learning.Sample{Features: features}).Next(nil)) {
			_ = *(*algo.Reading)(out)
		}

		if err := predicted.Error(); err != nil {
			b.Fatal(err)
		}

		step++
	}
}
