package mcts

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRewardStandardDeviation(t *testing.T) {
	Convey("Given a node accumulating economic rewards", t, func() {
		Convey("with fewer than two visits the deviation is undefined (zero)", func() {
			node := &SearchNode{}
			node.Visits = 1
			node.Mean = 5
			So(node.RewardStandardDeviation(), ShouldEqual, 0)
		})

		Convey("with two visits the sample deviation is the spread", func() {
			node := &SearchNode{}
			backpropagate(node, 3)
			backpropagate(node, 5)

			So(node.Visits, ShouldEqual, 2)
			So(node.MeanReward(), ShouldEqual, 4)
			// Sample variance = ((3-4)^2 + (5-4)^2)/(2-1) = 2.
			So(node.RewardStandardDeviation(), ShouldAlmostEqual, math.Sqrt(2), 1e-12)
		})

		Convey("with many visits the Welford accumulator matches the textbook sample deviation", func() {
			node := &SearchNode{}
			values := []float64{2, 4, 4, 4, 5, 5, 7, 9}

			for _, value := range values {
				backpropagate(node, value)
			}

			mean := 0.0

			for _, value := range values {
				mean += value
			}

			mean /= float64(len(values))
			sumSquares := 0.0

			for _, value := range values {
				sumSquares += (value - mean) * (value - mean)
			}

			expected := math.Sqrt(sumSquares / float64(len(values)-1))
			So(node.MeanReward(), ShouldAlmostEqual, mean, 1e-12)
			So(node.RewardStandardDeviation(), ShouldAlmostEqual, expected, 1e-12)
		})
	})
}

func TestStandardError(t *testing.T) {
	Convey("Given a node with observed rewards", t, func() {
		Convey("the standard error is the deviation over the square root of visits", func() {
			node := &SearchNode{}

			for _, value := range []float64{3, 4, 5, 6} {
				backpropagate(node, value)
			}

			expected := node.RewardStandardDeviation() / math.Sqrt(float64(node.Visits))
			So(node.StandardError(), ShouldAlmostEqual, expected, 1e-12)
		})

		Convey("with zero visits the standard error is zero", func() {
			node := &SearchNode{}
			So(node.StandardError(), ShouldEqual, 0)
		})
	})
}
