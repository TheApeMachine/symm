package causal

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRidgeSolve(t *testing.T) {
	Convey("Given a well-conditioned normal system", t, func() {
		normal := [][]float64{
			{4, 1},
			{1, 3},
		}
		vector := []float64{1, 2}

		coefficients, ok := ridgeSolve(normal, vector)

		Convey("It should recover a finite solution", func() {
			So(ok, ShouldBeTrue)
			So(len(coefficients), ShouldEqual, 2)
		})
	})
}

func TestConditionEstimate(t *testing.T) {
	Convey("Given a degenerate normal matrix", t, func() {
		normal := [][]float64{
			{1, 0},
			{0, 0},
		}

		Convey("It should report an infinite condition", func() {
			So(math.IsInf(conditionEstimate(normal), 1), ShouldBeTrue)
		})
	})
}

func BenchmarkRidgeSolve(b *testing.B) {
	normal := [][]float64{
		{4, 1, 0.2},
		{1, 3, 0.1},
		{0.2, 0.1, 2},
	}
	vector := []float64{1, 2, 0.5}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = ridgeSolve(normal, vector)
	}
}
