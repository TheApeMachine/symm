package distribution

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
evaluatePrimitive drives one one-shot distribution Primitive over one input
and returns its single output.
*/
func evaluatePrimitive[U any, T any](t *testing.T, operation core.Primitive, input T) U {
	t.Helper()

	outEval := transport.NewEvaluate(operation)
	var out U

	for res := range outEval.Next(transport.NewValues(input).Next(nil)) {
		out = *(*U)(res)
	}

	err := outEval.Error()

	if err != nil {
		t.Fatalf("distribution evaluation: %v", err)
	}

	return out
}

func TestNormalizeNext(t *testing.T) {
	Convey("Given a set of non-negative weights", t, func() {
		Convey("Normalize scales them to a unit sum and reports the total", func() {
			reading := evaluatePrimitive[NormalizedReading](t, NewNormalize(), WeightsInput{Weights: []float64{1, 1, 2}})

			So(reading.Total, ShouldEqual, 4)
			So(reading.Weights[0], ShouldAlmostEqual, 0.25)
			So(reading.Weights[1], ShouldAlmostEqual, 0.25)
			So(reading.Weights[2], ShouldAlmostEqual, 0.5)
		})

		Convey("negative weights are treated as zero", func() {
			reading := evaluatePrimitive[NormalizedReading](t, NewNormalize(), WeightsInput{Weights: []float64{-1, 1}})

			So(reading.Total, ShouldEqual, 1)
			So(reading.Weights[0], ShouldEqual, 0)
			So(reading.Weights[1], ShouldEqual, 1)
		})

		Convey("a zero total returns an all-zero slice and total 0", func() {
			reading := evaluatePrimitive[NormalizedReading](t, NewNormalize(), WeightsInput{Weights: []float64{0, 0}})

			So(reading.Total, ShouldEqual, 0)
			So(reading.Weights[0], ShouldEqual, 0)
			So(reading.Weights[1], ShouldEqual, 0)
		})
	})
}

func TestWasserstein1Next(t *testing.T) {
	Convey("Given two distributions over the same sorted support", t, func() {
		positions := []float64{0, 1, 2, 3}

		Convey("identical shapes have distance zero", func() {
			input := DistanceInput{Positions: positions, WeightsA: []float64{1, 2, 1, 0}, WeightsB: []float64{1, 2, 1, 0}}
			So(evaluatePrimitive[float64](t, NewWasserstein1(), input), ShouldAlmostEqual, 0)
		})

		Convey("the distance is the cumulative-mass discrepancy integrated over support", func() {
			// A concentrates all mass at position 0; B concentrates all mass at
			// position 3. The earth mover's distance is exactly 3 (the full
			// mass travels 3 units).
			input := DistanceInput{Positions: positions, WeightsA: []float64{1, 0, 0, 0}, WeightsB: []float64{0, 0, 0, 1}}
			So(evaluatePrimitive[float64](t, NewWasserstein1(), input), ShouldAlmostEqual, 3)
		})

		Convey("an empty or mismatched support returns +Inf", func() {
			So(math.IsInf(evaluatePrimitive[float64](t, NewWasserstein1(), DistanceInput{}), 1), ShouldBeTrue)

			mismatched := DistanceInput{Positions: []float64{0, 1}, WeightsA: []float64{1}, WeightsB: []float64{1}}
			So(math.IsInf(evaluatePrimitive[float64](t, NewWasserstein1(), mismatched), 1), ShouldBeTrue)
		})

		Convey("a zero-total distribution returns +Inf rather than fabricating a distance", func() {
			input := DistanceInput{Positions: positions, WeightsA: []float64{0, 0}, WeightsB: []float64{1, 1}}
			So(math.IsInf(evaluatePrimitive[float64](t, NewWasserstein1(), input), 1), ShouldBeTrue)
		})
	})
}

func TestKolmogorovSmirnovNext(t *testing.T) {
	Convey("Given two distributions over the same sorted support", t, func() {
		positions := []float64{0, 1, 2, 3}

		Convey("identical shapes have statistic zero", func() {
			input := DistanceInput{Positions: positions, WeightsA: []float64{1, 2, 1, 0}, WeightsB: []float64{1, 2, 1, 0}}
			So(evaluatePrimitive[float64](t, NewKolmogorovSmirnov(), input), ShouldAlmostEqual, 0)
		})

		Convey("disjointly supported shapes have statistic one", func() {
			input := DistanceInput{Positions: positions, WeightsA: []float64{4, 0, 0, 0}, WeightsB: []float64{0, 0, 0, 4}}
			So(evaluatePrimitive[float64](t, NewKolmogorovSmirnov(), input), ShouldAlmostEqual, 1)
		})

		Convey("the statistic is the supremum of cumulative disagreement", func() {
			// A: mass 0.5 at 0 and 0.5 at 2. B: mass 0.5 at 1 and 0.5 at 3.
			// CDF A: [.5,.5,1,1]; CDF B: [0,.5,.5,1].
			// Max |A-B| = .5, at position 0.
			input := DistanceInput{Positions: positions, WeightsA: []float64{2, 0, 2, 0}, WeightsB: []float64{0, 2, 0, 2}}
			So(evaluatePrimitive[float64](t, NewKolmogorovSmirnov(), input), ShouldAlmostEqual, 0.5)
		})

		Convey("an empty support returns +Inf", func() {
			So(math.IsInf(evaluatePrimitive[float64](t, NewKolmogorovSmirnov(), DistanceInput{}), 1), ShouldBeTrue)
		})
	})
}

func TestEntropyNext(t *testing.T) {
	Convey("Given normalized weights", t, func() {
		Convey("a single monopolized position has entropy zero", func() {
			So(evaluatePrimitive[float64](t, NewEntropy(), ShapeInput{Weights: []float64{1}}), ShouldAlmostEqual, 0)
		})

		Convey("a uniform distribution over n positions has entropy ln(n)", func() {
			So(evaluatePrimitive[float64](t, NewEntropy(), ShapeInput{Weights: []float64{0.5, 0.5}}), ShouldAlmostEqual, math.Log(2))
			So(evaluatePrimitive[float64](t, NewEntropy(), ShapeInput{Weights: []float64{0.25, 0.25, 0.25, 0.25}}), ShouldAlmostEqual, math.Log(4))
		})

		Convey("an empty distribution has entropy zero", func() {
			So(evaluatePrimitive[float64](t, NewEntropy(), ShapeInput{}), ShouldAlmostEqual, 0)
		})
	})
}

func TestConcentrationNext(t *testing.T) {
	Convey("Given normalized weights", t, func() {
		Convey("a single monopolized position has concentration one", func() {
			So(evaluatePrimitive[float64](t, NewConcentration(), ShapeInput{Weights: []float64{1}}), ShouldAlmostEqual, 1)
		})

		Convey("a uniform distribution over n positions has concentration 1/n", func() {
			So(evaluatePrimitive[float64](t, NewConcentration(), ShapeInput{Weights: []float64{0.5, 0.5}}), ShouldAlmostEqual, 0.5)
			So(evaluatePrimitive[float64](t, NewConcentration(), ShapeInput{Weights: []float64{0.25, 0.25, 0.25, 0.25}}), ShouldAlmostEqual, 0.25)
		})
	})
}

func TestSortedPositionsNext(t *testing.T) {
	Convey("Given unsorted positions paired with weights", t, func() {
		Convey("SortedPositions returns both sorted by position", func() {
			reading := evaluatePrimitive[SortedReading](t, NewSortedPositions(), SortedInput{
				Positions: []float64{3, 1, 2},
				Weights:   []float64{30, 10, 20},
			})

			So(reading.Positions, ShouldResemble, []float64{1, 2, 3})
			So(reading.Weights, ShouldResemble, []float64{10, 20, 30})
		})

		Convey("a mismatched length returns empty slices", func() {
			reading := evaluatePrimitive[SortedReading](t, NewSortedPositions(), SortedInput{
				Positions: []float64{1, 2},
				Weights:   []float64{1},
			})

			So(reading.Positions, ShouldBeNil)
			So(reading.Weights, ShouldBeNil)
		})
	})
}

func TestWasserstein1PairsNext(t *testing.T) {
	Convey("Given two sorted point streams on different supports", t, func() {
		Convey("identical single-point streams have distance zero", func() {
			input := PairsInput{
				Left:  []WeightedPoint{{Position: 0.5, Weight: 1}},
				Right: []WeightedPoint{{Position: 0.5, Weight: 1}},
			}

			So(evaluatePrimitive[float64](t, NewWasserstein1Pairs(), input), ShouldAlmostEqual, 0)
		})

		Convey("mirrored-but-equal mass profiles on the folded axis have distance zero", func() {
			input := PairsInput{
				Left:  []WeightedPoint{{Position: 0.5, Weight: 2}, {Position: 1.5, Weight: 1}},
				Right: []WeightedPoint{{Position: 0.5, Weight: 2}, {Position: 1.5, Weight: 1}},
			}

			So(evaluatePrimitive[float64](t, NewWasserstein1Pairs(), input), ShouldAlmostEqual, 0)
		})

		Convey("disjoint supports transport the full mass across the gap", func() {
			input := PairsInput{
				Left:  []WeightedPoint{{Position: 0, Weight: 1}},
				Right: []WeightedPoint{{Position: 3, Weight: 1}},
			}

			So(evaluatePrimitive[float64](t, NewWasserstein1Pairs(), input), ShouldAlmostEqual, 3)
		})

		Convey("Weighting does not affect a shared single point", func() {
			input := PairsInput{
				Left:  []WeightedPoint{{Position: 1, Weight: 100}},
				Right: []WeightedPoint{{Position: 1, Weight: 1}},
			}

			So(evaluatePrimitive[float64](t, NewWasserstein1Pairs(), input), ShouldAlmostEqual, 0)
		})

		Convey("a zero-total stream returns +Inf", func() {
			input := PairsInput{
				Left:  []WeightedPoint{{0, 0}},
				Right: []WeightedPoint{{0, 1}},
			}

			So(math.IsInf(evaluatePrimitive[float64](t, NewWasserstein1Pairs(), input), 1), ShouldBeTrue)
		})
	})
}

func TestKolmogorovSmirnovPairsNext(t *testing.T) {
	Convey("Given two sorted point streams", t, func() {
		Convey("equal streams have statistic zero", func() {
			input := PairsInput{
				Left:  []WeightedPoint{{Position: 0.5, Weight: 1}, {Position: 1, Weight: 1}},
				Right: []WeightedPoint{{Position: 0.5, Weight: 1}, {Position: 1, Weight: 1}},
			}

			So(evaluatePrimitive[float64](t, NewKolmogorovSmirnovPairs(), input), ShouldAlmostEqual, 0)
		})

		Convey("disjoint supports have statistic one", func() {
			input := PairsInput{
				Left:  []WeightedPoint{{Position: 0, Weight: 1}},
				Right: []WeightedPoint{{Position: 3, Weight: 1}},
			}

			So(evaluatePrimitive[float64](t, NewKolmogorovSmirnovPairs(), input), ShouldAlmostEqual, 1)
		})

		Convey("a zero-total stream returns +Inf", func() {
			input := PairsInput{
				Left:  []WeightedPoint{{0, 0}},
				Right: []WeightedPoint{{0, 1}},
			}

			So(math.IsInf(evaluatePrimitive[float64](t, NewKolmogorovSmirnovPairs(), input), 1), ShouldBeTrue)
		})
	})
}

func TestConcentrationPointsNext(t *testing.T) {
	Convey("Given a point stream", t, func() {
		Convey("a single point has concentration one", func() {
			So(evaluatePrimitive[float64](t, NewConcentrationPoints(), PointsInput{Points: []WeightedPoint{{Position: 1, Weight: 5}}}), ShouldAlmostEqual, 1)
		})

		Convey("two equal points have concentration 1/2", func() {
			input := PointsInput{Points: []WeightedPoint{{Position: 1, Weight: 3}, {Position: 2, Weight: 3}}}
			So(evaluatePrimitive[float64](t, NewConcentrationPoints(), input), ShouldAlmostEqual, 0.5)
		})

		Convey("a zero-total stream is empty", func() {
			So(evaluatePrimitive[float64](t, NewConcentrationPoints(), PointsInput{Points: []WeightedPoint{{Position: 1, Weight: 0}}}), ShouldAlmostEqual, 0)
		})
	})
}

func TestEntropyPointsNext(t *testing.T) {
	Convey("Given a point stream", t, func() {
		Convey("a single point has entropy zero", func() {
			So(evaluatePrimitive[float64](t, NewEntropyPoints(), PointsInput{Points: []WeightedPoint{{Position: 1, Weight: 5}}}), ShouldAlmostEqual, 0)
		})

		Convey("two equal points have entropy ln 2", func() {
			input := PointsInput{Points: []WeightedPoint{{Position: 1, Weight: 3}, {Position: 2, Weight: 3}}}
			So(evaluatePrimitive[float64](t, NewEntropyPoints(), input), ShouldAlmostEqual, math.Log(2))
		})

		Convey("a zero-total stream is empty", func() {
			So(evaluatePrimitive[float64](t, NewEntropyPoints(), PointsInput{Points: []WeightedPoint{{Position: 1, Weight: 0}}}), ShouldAlmostEqual, 0)
		})
	})
}
