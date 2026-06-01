package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBookLiquidity(t *testing.T) {
	Convey("Given book liquidity from spread and volume", t, func() {
		Convey("It should return zero for invalid inputs", func() {
			So(bookLiquidity(0, 100), ShouldEqual, 0)
			So(bookLiquidity(5, 0), ShouldEqual, 0)
		})

		Convey("It should floor ultra-tight spreads before dividing", func() {
			tight := bookLiquidity(0.01, 1000)
			floored := bookLiquidity(minSpreadBPSFloor, 1000)

			So(tight, ShouldEqual, floored)
			So(tight, ShouldAlmostEqual, 2000, 0.01)
		})
	})
}

func TestNewCausalSample(t *testing.T) {
	Convey("Given DAG node values", t, func() {
		sample := newCausalSample(0.2, 50, 3, 0.01)

		Convey("It should index nodes in DAG order", func() {
			So(sample.value(macroMomentumNode), ShouldEqual, 0.2)
			So(sample.value(liquidityNode), ShouldEqual, 50)
			So(sample.value(localFlowNode), ShouldEqual, 3)
			So(sample.value(priceVelocityNode), ShouldEqual, 0.01)
		})
	})
}

func BenchmarkBookLiquidity(b *testing.B) {
	for b.Loop() {
		_ = bookLiquidity(12, 500)
	}
}
