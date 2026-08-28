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
