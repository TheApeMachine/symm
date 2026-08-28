package distribution

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNormalize(t *testing.T) {
	Convey("Given a set of non-negative weights", t, func() {
		Convey("Normalize scales them to a unit sum and reports the total", func() {
			weights, total := Normalize([]float64{1, 1, 2})

			So(total, ShouldEqual, 4)
			So(weights[0], ShouldAlmostEqual, 0.25)
			So(weights[1], ShouldAlmostEqual, 0.25)
			So(weights[2], ShouldAlmostEqual, 0.5)
		})

		Convey("negative weights are treated as zero", func() {
			weights, total := Normalize([]float64{-1, 1})

			So(total, ShouldEqual, 1)
			So(weights[0], ShouldEqual, 0)
			So(weights[1], ShouldEqual, 1)
		})

		Convey("a zero total returns an all-zero slice and total 0", func() {
			weights, total := Normalize([]float64{0, 0})

			So(total, ShouldEqual, 0)
			So(weights[0], ShouldEqual, 0)
			So(weights[1], ShouldEqual, 0)
		})
	})
}

func TestWasserstein1(t *testing.T) {
	Convey("Given two distributions over the same sorted support", t, func() {
		positions := []float64{0, 1, 2, 3}

		Convey("identical shapes have distance zero", func() {
			So(Wasserstein1(positions, []float64{1, 2, 1, 0}, []float64{1, 2, 1, 0}), ShouldAlmostEqual, 0)
		})

		Convey("the distance is the cumulative-mass discrepancy integrated over support", func() {
			// A concentrates all mass at position 0; B concentrates all mass at
			// position 3. The earth mover's distance is exactly 3 (the full
			// mass travels 3 units).
			So(Wasserstein1(positions, []float64{1, 0, 0, 0}, []float64{0, 0, 0, 1}), ShouldAlmostEqual, 3)
		})

		Convey("an empty or mismatched support returns +Inf", func() {
			So(math.IsInf(Wasserstein1(nil, nil, nil), 1), ShouldBeTrue)
			So(math.IsInf(Wasserstein1([]float64{0, 1}, []float64{1}, []float64{1}), 1), ShouldBeTrue)
		})

		Convey("a zero-total distribution returns +Inf rather than fabricating a distance", func() {
			So(math.IsInf(Wasserstein1(positions, []float64{0, 0}, []float64{1, 1}), 1), ShouldBeTrue)
		})
	})
}

func TestKolmogorovSmirnov(t *testing.T) {
	Convey("Given two distributions over the same sorted support", t, func() {
		positions := []float64{0, 1, 2, 3}

		Convey("identical shapes have statistic zero", func() {
			So(KolmogorovSmirnov(positions, []float64{1, 2, 1, 0}, []float64{1, 2, 1, 0}), ShouldAlmostEqual, 0)
		})

		Convey("disjointly supported shapes have statistic one", func() {
			So(KolmogorovSmirnov(positions, []float64{4, 0, 0, 0}, []float64{0, 0, 0, 4}), ShouldAlmostEqual, 1)
		})

		Convey("the statistic is the supremum of cumulative disagreement", func() {
			// A: mass 0.5 at 0 and 0.5 at 2. B: mass 0.5 at 1 and 0.5 at 3.
			// CDF A: [.5,.5,1,1]; CDF B: [0,.5,.5,1].
			// Max |A-B| = .5, at position 0.
			So(KolmogorovSmirnov(positions, []float64{2, 0, 2, 0}, []float64{0, 2, 0, 2}), ShouldAlmostEqual, 0.5)
		})

		Convey("an empty support returns +Inf", func() {
			So(math.IsInf(KolmogorovSmirnov(nil, nil, nil), 1), ShouldBeTrue)
		})
	})
}

func TestEntropy(t *testing.T) {
	Convey("Given normalized weights", t, func() {
		Convey("a single monopolized position has entropy zero", func() {
			So(Entropy([]float64{1}), ShouldAlmostEqual, 0)
		})

		Convey("a uniform distribution over n positions has entropy ln(n)", func() {
			So(Entropy([]float64{0.5, 0.5}), ShouldAlmostEqual, math.Log(2))
			So(Entropy([]float64{0.25, 0.25, 0.25, 0.25}), ShouldAlmostEqual, math.Log(4))
		})

		Convey("an empty distribution has entropy zero", func() {
			So(Entropy(nil), ShouldAlmostEqual, 0)
		})
	})
}

func TestConcentration(t *testing.T) {
	Convey("Given normalized weights", t, func() {
		Convey("a single monopolized position has concentration one", func() {
			So(Concentration([]float64{1}), ShouldAlmostEqual, 1)
		})

		Convey("a uniform distribution over n positions has concentration 1/n", func() {
			So(Concentration([]float64{0.5, 0.5}), ShouldAlmostEqual, 0.5)
			So(Concentration([]float64{0.25, 0.25, 0.25, 0.25}), ShouldAlmostEqual, 0.25)
		})
	})
}

func TestSortedPositions(t *testing.T) {
	Convey("Given unsorted positions paired with weights", t, func() {
		Convey("SortedPositions returns both sorted by position", func() {
			positions, weights := SortedPositions(
				[]float64{3, 1, 2},
				[]float64{30, 10, 20},
			)

			So(positions, ShouldResemble, []float64{1, 2, 3})
			So(weights, ShouldResemble, []float64{10, 20, 30})
		})

		Convey("a mismatched length returns nil", func() {
			positions, weights := SortedPositions([]float64{1, 2}, []float64{1})

			So(positions, ShouldBeNil)
			So(weights, ShouldBeNil)
		})
	})
}

func TestWasserstein1Pairs(t *testing.T) {
	Convey("Given two sorted point streams on different supports", t, func() {
		Convey("identical single-point streams have distance zero", func() {
			left := []WeightedPoint{{Position: 0.5, Weight: 1}}
			right := []WeightedPoint{{Position: 0.5, Weight: 1}}

			So(Wasserstein1Pairs(left, right), ShouldAlmostEqual, 0)
		})

		Convey("mirrored-but-equal mass profiles on the folded axis have distance zero", func() {
			left := []WeightedPoint{{Position: 0.5, Weight: 2}, {Position: 1.5, Weight: 1}}
			right := []WeightedPoint{{Position: 0.5, Weight: 2}, {Position: 1.5, Weight: 1}}

			So(Wasserstein1Pairs(left, right), ShouldAlmostEqual, 0)
		})

		Convey("disjoint supports transport the full mass across the gap", func() {
			left := []WeightedPoint{{Position: 0, Weight: 1}}
			right := []WeightedPoint{{Position: 3, Weight: 1}}

			So(Wasserstein1Pairs(left, right), ShouldAlmostEqual, 3)
		})

		Convey("Weighting does not affect a shared single point", func() {
			left := []WeightedPoint{{Position: 1, Weight: 100}}
			right := []WeightedPoint{{Position: 1, Weight: 1}}

			So(Wasserstein1Pairs(left, right), ShouldAlmostEqual, 0)
		})

		Convey("a zero-total stream returns +Inf", func() {
			So(math.IsInf(Wasserstein1Pairs([]WeightedPoint{{0, 0}}, []WeightedPoint{{0, 1}}), 1), ShouldBeTrue)
		})
	})
}

func TestKolmogorovSmirnovPairs(t *testing.T) {
	Convey("Given two sorted point streams", t, func() {
		Convey("equal streams have statistic zero", func() {
			left := []WeightedPoint{{Position: 0.5, Weight: 1}, {Position: 1, Weight: 1}}
			right := []WeightedPoint{{Position: 0.5, Weight: 1}, {Position: 1, Weight: 1}}

			So(KolmogorovSmirnovPairs(left, right), ShouldAlmostEqual, 0)
		})

		Convey("disjoint supports have statistic one", func() {
			left := []WeightedPoint{{Position: 0, Weight: 1}}
			right := []WeightedPoint{{Position: 3, Weight: 1}}

			So(KolmogorovSmirnovPairs(left, right), ShouldAlmostEqual, 1)
		})

		Convey("a zero-total stream returns +Inf", func() {
			So(math.IsInf(KolmogorovSmirnovPairs([]WeightedPoint{{0, 0}}, []WeightedPoint{{0, 1}}), 1), ShouldBeTrue)
		})
	})
}

func TestConcentrationPointsAndEntropyPoints(t *testing.T) {
	Convey("Given a point stream", t, func() {
		Convey("a single point has concentration one and entropy zero", func() {
			points := []WeightedPoint{{Position: 1, Weight: 5}}

			So(ConcentrationPoints(points), ShouldAlmostEqual, 1)
			So(EntropyPoints(points), ShouldAlmostEqual, 0)
		})

		Convey("two equal points have concentration 1/2 and entropy ln 2", func() {
			points := []WeightedPoint{{Position: 1, Weight: 3}, {Position: 2, Weight: 3}}

			So(ConcentrationPoints(points), ShouldAlmostEqual, 0.5)
			So(EntropyPoints(points), ShouldAlmostEqual, math.Log(2))
		})

		Convey("a zero-total stream is empty", func() {
			points := []WeightedPoint{{Position: 1, Weight: 0}}

			So(ConcentrationPoints(points), ShouldAlmostEqual, 0)
			So(EntropyPoints(points), ShouldAlmostEqual, 0)
		})
	})
}
