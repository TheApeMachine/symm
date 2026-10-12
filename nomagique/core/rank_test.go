package core

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
exactRank counts the values below and equal to value by brute force.
*/
func exactRank(values []float64, value float64) (below float64, equal float64) {
	for _, item := range values {
		if item < value {
			below++
		}

		if item == value {
			equal++
		}
	}

	return below, equal
}

/*
retained is the number of items the summary physically holds.
*/
func retained(ranks *Ranks) int {
	held := 0

	for _, level := range ranks.levels {
		held += len(level)
	}

	return held
}

/*
stream draws a reproducible heavy-tailed stream with ties: a lognormal
quantized to a coarse grid, the shape of a tick-quantized market series.
*/
func stream(seed uint64, length int) []float64 {
	random := rand.New(rand.NewPCG(seed, seed))
	values := make([]float64, length)

	for index := range values {
		values[index] = math.Round(math.Exp(random.NormFloat64()) * 100)
	}

	return values
}

func TestNewRanks(t *testing.T) {
	Convey("Given a new summary", t, func() {
		ranks := NewRanks(7)

		Convey("it holds nothing and has no error", func() {
			below, equal := ranks.Rank(1)
			So(ranks.Count(), ShouldEqual, 0)
			So(ranks.Variance(), ShouldEqual, 0)
			So(below, ShouldEqual, 0)
			So(equal, ShouldEqual, 0)
		})
	})
}

func TestRanksRank(t *testing.T) {
	Convey("Given a stream too short to afford any compaction", t, func() {
		ranks := NewRanks(11)
		values := stream(11, 40)

		for _, value := range values {
			ranks.Add(value)
		}

		Convey("every rank is exact, ties included", func() {
			So(ranks.Variance(), ShouldEqual, 0)

			for _, query := range append(values, -1, math.MaxFloat64) {
				below, equal := ranks.Rank(query)
				wantBelow, wantEqual := exactRank(values, query)
				So(below, ShouldEqual, wantBelow)
				So(equal, ShouldEqual, wantEqual)
			}
		})
	})
}

func TestRanksAdd(t *testing.T) {
	Convey("Given a long tick-quantized stream", t, func() {
		values := stream(23, 200000)
		ranks := NewRanks(23)
		replay := NewRanks(23)

		for _, value := range values {
			ranks.Add(value)
			replay.Add(value)
		}

		Convey("the held weight always equals the observation count", func() {
			below, equal := ranks.Rank(math.MaxFloat64)
			So(below+equal, ShouldEqual, float64(len(values)))
			So(ranks.Count(), ShouldEqual, float64(len(values)))
		})

		Convey("the error variance stays within the tolerance budget", func() {
			So(ranks.Variance(), ShouldBeGreaterThan, 0)
			So(ranks.Variance(), ShouldBeLessThanOrEqualTo, Tolerance*Tolerance*ranks.Count()/4)
		})

		Convey("the summary is far smaller than the stream", func() {
			So(retained(&ranks), ShouldBeLessThan, len(values)/int(MinimumRankPrior))
		})

		Convey("the same seed and order reproduce the same summary", func() {
			So(replay.levels, ShouldResemble, ranks.levels)
			So(replay.Variance(), ShouldEqual, ranks.Variance())
		})

		Convey("every rank error is below the empirical distribution's own sampling error", func() {
			sorted := slices.Clone(values)
			slices.Sort(sorted)
			sampling := math.Sqrt(ranks.Count() / 4)

			for index := 0; index < len(sorted); index += len(sorted) / int(MinimumRankPrior) {
				query := sorted[index]
				below, equal := ranks.Rank(query)
				wantBelow, wantEqual := exactRank(values, query)
				So(math.Abs(below-wantBelow), ShouldBeLessThanOrEqualTo, sampling)
				So(math.Abs(below+equal-wantBelow-wantEqual), ShouldBeLessThanOrEqualTo, sampling)
			}
		})
	})
}
