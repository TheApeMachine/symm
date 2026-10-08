package probability_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestDistributionSnapshot(t *testing.T) {
	Convey("A softmax simplex preserves winner, confidence and ambiguity", t, func() {
		var first probability.Reading

		for _, run := range [][]float64{{1, 1, 1, 1}, {0, 5, 1}, {1000, 1001}, {8}, {0, 4}} {
			node := probability.NewDistribution()
			outEval := transport.NewEvaluate(node)
			var out probability.Reading

			for res := range outEval.Next(transport.NewValues(run...).Next(nil)) {
				out = *(*probability.Reading)(res)
			}

			err := outEval.Error()
			So(err, ShouldBeNil)

			maximum := run[0]
			expected := make([]float64, len(run))
			total := 0.0
			winner := 0

			for index, value := range run {
				if value > maximum {
					maximum = value
				}

				if value > run[winner] {
					winner = index
				}
			}

			for index, value := range run {
				expected[index] = math.Exp(value - maximum)
				total += expected[index]
			}

			entropy := 0.0

			for index := range expected {
				expected[index] /= total
				So(out.Probabilities[index], ShouldAlmostEqual, expected[index])

				if expected[index] > 0 {
					entropy -= expected[index] * math.Log(expected[index])
				}
			}

			ambiguity := 0.0

			if len(run) > 1 {
				ambiguity = entropy / math.Log(float64(len(run)))
			}

			So(out.Winner, ShouldEqual, winner)
			So(out.Confidence, ShouldAlmostEqual, expected[winner])
			So(out.Ambiguity, ShouldAlmostEqual, ambiguity)
			So(out.Sharpness, ShouldAlmostEqual, 1-ambiguity)

			if first.Probabilities == nil {
				first = out
			}
		}

		So(first.Ambiguity, ShouldEqual, 1)
		So(first.Confidence, ShouldEqual, .25)
	})
}

func TestDistributionUndefinedInput(t *testing.T) {
	Convey("Empty or non-finite logits fail instead of inventing a simplex", t, func() {
		for _, members := range [][]float64{nil, {math.NaN()}, {math.Inf(1)}} {
			node := probability.NewDistribution()
			_Eval := transport.NewEvaluate(node)

			for range _Eval.Next(transport.NewValues(members...).Next(nil)) {
			}

			err := _Eval.Error()
			So(err, ShouldNotBeNil)
			So(node.Error(), ShouldNotBeNil)
		}
	})
}

func BenchmarkNewDistribution(b *testing.B) {
	node := probability.NewDistribution()
	b.ReportAllocs()

	for b.Loop() {
		out := tests.CollectSeq[probability.Reading](node.Next(transport.NewValues(0.0, 5.0, 1.0).Next(nil)))

		if len(out) != 1 || node.Error() != nil {
			b.Fatal("expected one distribution", node.Error())
		}
	}
}

func TestDistributionNext(t *testing.T) {
	Convey("Independent softmax runs keep a unit simplex", t, func() {
		for _, values := range [][]float64{{1, 1, 1, 1}, {-1000, 1000, 0}, {8}, {0, 5, 1}} {
			outEval := transport.NewEvaluate(probability.NewDistribution())
			var out probability.Reading

			for res := range outEval.Next(transport.NewValues(values...).Next(nil)) {
				out = *(*probability.Reading)(res)
			}

			err := outEval.Error()
			So(err, ShouldBeNil)
			sum := 0.0

			for _, value := range out.Probabilities {
				sum += value
			}

			So(sum, ShouldAlmostEqual, 1)
			maximum := values[0]
			winner := 0
			total := 0.0

			for index, value := range values {
				if value > maximum {
					maximum = value
					winner = index
				}
			}

			for _, value := range values {
				total += math.Exp(value - maximum)
			}

			So(out.Winner, ShouldEqual, winner)
			So(out.Confidence, ShouldAlmostEqual, 1/total)
		}
	})
}
