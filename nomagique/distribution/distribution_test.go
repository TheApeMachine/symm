package distribution_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestNormalizeNext(t *testing.T) {
	Convey("Given a set of non-negative weights", t, func() {
		Convey("Normalize scales them to a unit sum and reports the total", func() {
			node := distribution.NewNormalize()
			out := tests.CollectSeq[[2][]float64](node.Next(data.NewValue([]float64{1, 1, 2}).Next(nil)))

			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0][1][0], ShouldEqual, 4)
			So(out[0][0][0], ShouldAlmostEqual, 0.25)
			So(out[0][0][1], ShouldAlmostEqual, 0.25)
			So(out[0][0][2], ShouldAlmostEqual, 0.5)
		})

		Convey("negative weights are treated as zero", func() {
			node := distribution.NewNormalize()
			out := tests.CollectSeq[[2][]float64](node.Next(data.NewValue([]float64{-1, 1}).Next(nil)))

			So(out[0][1][0], ShouldEqual, 1)
			So(out[0][0], ShouldResemble, []float64{0, 1})
		})

		Convey("a zero total returns an all-zero slice and total 0", func() {
			node := distribution.NewNormalize()
			out := tests.CollectSeq[[2][]float64](node.Next(data.NewValue([]float64{0, 0}).Next(nil)))

			So(out[0][1][0], ShouldEqual, 0)
			So(out[0][0], ShouldResemble, []float64{0, 0})
		})
	})
}

func TestWasserstein1Next(t *testing.T) {
	Convey("Given two distributions over the same sorted support", t, func() {
		positions := []float64{0, 1, 2, 3}

		Convey("identical shapes have distance zero", func() {
			node := distribution.NewWasserstein1()
			out := tests.CollectSeq[float64](node.Next(data.NewValue([3][]float64{positions, {1, 2, 1, 0}, {1, 2, 1, 0}}).Next(nil)))
			So(node.Error(), ShouldBeNil)
			So(out[0], ShouldAlmostEqual, 0)
		})

		Convey("the distance is the cumulative-mass discrepancy integrated over support", func() {
			node := distribution.NewWasserstein1()
			out := tests.CollectSeq[float64](node.Next(data.NewValue([3][]float64{positions, {1, 0, 0, 0}, {0, 0, 0, 1}}).Next(nil)))
			So(out[0], ShouldAlmostEqual, 3)
		})

		Convey("an empty or mismatched support returns +Inf", func() {
			node := distribution.NewWasserstein1()
			out := tests.CollectSeq[float64](node.Next(data.NewValue(
				[3][]float64{},
				[3][]float64{{0, 1}, {1}, {1}},
			).Next(nil)))
			So(len(out), ShouldEqual, 2)
			So(math.IsInf(out[0], 1), ShouldBeTrue)
			So(math.IsInf(out[1], 1), ShouldBeTrue)
		})

		Convey("a zero-total distribution returns +Inf rather than fabricating a distance", func() {
			node := distribution.NewWasserstein1()
			out := tests.CollectSeq[float64](node.Next(data.NewValue([3][]float64{positions, {0, 0, 0, 0}, {1, 1, 1, 1}}).Next(nil)))
			So(math.IsInf(out[0], 1), ShouldBeTrue)
		})
	})
}

func TestKolmogorovSmirnovNext(t *testing.T) {
	Convey("Given two distributions over the same sorted support", t, func() {
		positions := []float64{0, 1, 2, 3}

		Convey("identical shapes have statistic zero", func() {
			node := distribution.NewKolmogorovSmirnov()
			out := tests.CollectSeq[float64](node.Next(data.NewValue([3][]float64{positions, {1, 2, 1, 0}, {1, 2, 1, 0}}).Next(nil)))
			So(node.Error(), ShouldBeNil)
			So(out[0], ShouldAlmostEqual, 0)
		})

		Convey("disjointly supported shapes have statistic one", func() {
			node := distribution.NewKolmogorovSmirnov()
			out := tests.CollectSeq[float64](node.Next(data.NewValue([3][]float64{positions, {4, 0, 0, 0}, {0, 0, 0, 4}}).Next(nil)))
			So(out[0], ShouldAlmostEqual, 1)
		})

		Convey("the statistic is the supremum of cumulative disagreement", func() {
			// CDF A: [.5,.5,1,1]; CDF B: [0,.5,.5,1]. Max |A-B| = .5.
			node := distribution.NewKolmogorovSmirnov()
			out := tests.CollectSeq[float64](node.Next(data.NewValue([3][]float64{positions, {2, 0, 2, 0}, {0, 2, 0, 2}}).Next(nil)))
			So(out[0], ShouldAlmostEqual, 0.5)
		})

		Convey("an empty support returns +Inf", func() {
			node := distribution.NewKolmogorovSmirnov()
			out := tests.CollectSeq[float64](node.Next(data.NewValue([3][]float64{}).Next(nil)))
			So(math.IsInf(out[0], 1), ShouldBeTrue)
		})
	})
}

func TestEntropyNext(t *testing.T) {
	Convey("Given normalized weights", t, func() {
		node := distribution.NewEntropy()
		out := tests.CollectSeq[float64](node.Next(data.NewValue(
			[]float64{1},
			[]float64{0.5, 0.5},
			[]float64{0.25, 0.25, 0.25, 0.25},
			[]float64{},
		).Next(nil)))

		So(node.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 4)
		So(out[0], ShouldAlmostEqual, 0)
		So(out[1], ShouldAlmostEqual, math.Log(2))
		So(out[2], ShouldAlmostEqual, math.Log(4))
		So(out[3], ShouldAlmostEqual, 0)
	})
}

func TestConcentrationNext(t *testing.T) {
	Convey("Given normalized weights", t, func() {
		node := distribution.NewConcentration()
		out := tests.CollectSeq[float64](node.Next(data.NewValue(
			[]float64{1},
			[]float64{0.5, 0.5},
			[]float64{0.25, 0.25, 0.25, 0.25},
		).Next(nil)))

		So(node.Error(), ShouldBeNil)
		So(out[0], ShouldAlmostEqual, 1)
		So(out[1], ShouldAlmostEqual, 0.5)
		So(out[2], ShouldAlmostEqual, 0.25)
	})
}

func TestSortedPositionsNext(t *testing.T) {
	Convey("Given unsorted positions paired with weights", t, func() {
		Convey("SortedPositions returns both sorted by position", func() {
			node := distribution.NewSortedPositions()
			out := tests.CollectSeq[[2][]float64](node.Next(data.NewValue([2][]float64{{3, 1, 2}, {30, 10, 20}}).Next(nil)))

			So(node.Error(), ShouldBeNil)
			So(out[0][0], ShouldResemble, []float64{1, 2, 3})
			So(out[0][1], ShouldResemble, []float64{10, 20, 30})
		})

		Convey("a mismatched length returns empty slices", func() {
			node := distribution.NewSortedPositions()
			out := tests.CollectSeq[[2][]float64](node.Next(data.NewValue([2][]float64{{1, 2}, {1}}).Next(nil)))

			So(out[0][0], ShouldBeNil)
			So(out[0][1], ShouldBeNil)
		})
	})
}

func TestMergedWalkNext(t *testing.T) {
	Convey("MergedWalk reports KS, W1, and distinct positions in one pass", t, func() {
		node := distribution.NewMergedWalk()
		out := tests.CollectSeq[[3]float64](node.Next(data.NewValue([2][][2]float64{
			{{0, 1}, {2, 1}},
			{{1, 1}, {2, 1}},
		}).Next(nil)))

		So(node.Error(), ShouldBeNil)
		So(out[0][0], ShouldAlmostEqual, 0.5)
		So(out[0][1], ShouldAlmostEqual, 0.5)
		So(out[0][2], ShouldEqual, 3)
	})
}

func TestWasserstein1PairsNext(t *testing.T) {
	Convey("Given two sorted point streams on different supports", t, func() {
		node := distribution.NewWasserstein1Pairs()
		out := tests.CollectSeq[float64](node.Next(data.NewValue(
			[2][][2]float64{{{0.5, 1}}, {{0.5, 1}}},
			[2][][2]float64{{{0.5, 2}, {1.5, 1}}, {{0.5, 2}, {1.5, 1}}},
			[2][][2]float64{{{0, 1}}, {{3, 1}}},
			[2][][2]float64{{{1, 100}}, {{1, 1}}},
			[2][][2]float64{{{0, 0}}, {{0, 1}}},
		).Next(nil)))

		So(node.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 5)
		So(out[0], ShouldAlmostEqual, 0)
		So(out[1], ShouldAlmostEqual, 0)
		So(out[2], ShouldAlmostEqual, 3)
		So(out[3], ShouldAlmostEqual, 0)
		So(math.IsInf(out[4], 1), ShouldBeTrue)
	})
}

func TestKolmogorovSmirnovPairsNext(t *testing.T) {
	Convey("Given two sorted point streams", t, func() {
		node := distribution.NewKolmogorovSmirnovPairs()
		out := tests.CollectSeq[float64](node.Next(data.NewValue(
			[2][][2]float64{{{0.5, 1}, {1, 1}}, {{0.5, 1}, {1, 1}}},
			[2][][2]float64{{{0, 1}}, {{3, 1}}},
			[2][][2]float64{{{0, 0}}, {{0, 1}}},
		).Next(nil)))

		So(node.Error(), ShouldBeNil)
		So(out[0], ShouldAlmostEqual, 0)
		So(out[1], ShouldAlmostEqual, 1)
		So(math.IsInf(out[2], 1), ShouldBeTrue)
	})
}

func TestConcentrationPointsNext(t *testing.T) {
	Convey("Given a point stream", t, func() {
		node := distribution.NewConcentrationPoints()
		out := tests.CollectSeq[float64](node.Next(data.NewValue(
			[][2]float64{{1, 5}},
			[][2]float64{{1, 3}, {2, 3}},
			[][2]float64{{1, 0}},
		).Next(nil)))

		So(node.Error(), ShouldBeNil)
		So(out[0], ShouldAlmostEqual, 1)
		So(out[1], ShouldAlmostEqual, 0.5)
		So(out[2], ShouldAlmostEqual, 0)
	})
}

func TestEntropyPointsNext(t *testing.T) {
	Convey("Given a point stream", t, func() {
		node := distribution.NewEntropyPoints()
		out := tests.CollectSeq[float64](node.Next(data.NewValue(
			[][2]float64{{1, 5}},
			[][2]float64{{1, 3}, {2, 3}},
			[][2]float64{{1, 0}},
		).Next(nil)))

		So(node.Error(), ShouldBeNil)
		So(out[0], ShouldAlmostEqual, 0)
		So(out[1], ShouldAlmostEqual, math.Log(2))
		So(out[2], ShouldAlmostEqual, 0)
	})
}
