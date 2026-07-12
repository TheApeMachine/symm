package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCoordinateStatisticsMeasure(t *testing.T) {
	Convey("Given the same population in different order-ID iteration orders", t, func() {
		first := []*PhysicalOrder{
			{OrderID: "z", LimitPrice: 99, Quantity: 1},
			{OrderID: "a", LimitPrice: 101, Quantity: 4},
			{OrderID: "m", LimitPrice: 102, Quantity: 16},
		}
		second := []*PhysicalOrder{first[2], first[0], first[1]}
		left := CoordinateStatistics{}
		right := CoordinateStatistics{}

		leftReady := left.Measure(first, 100, 1e-9)
		rightReady := right.Measure(second, 100, 1e-9)

		Convey("It should derive identical robust epoch statistics", func() {
			So(leftReady, ShouldBeTrue)
			So(rightReady, ShouldBeTrue)
			So(left, ShouldResemble, right)
		})
	})
}

func BenchmarkCoordinateStatisticsMeasure(b *testing.B) {
	orders := make([]*PhysicalOrder, 128)

	for index := range orders {
		orders[index] = &PhysicalOrder{
			LimitPrice: 99 + float64(index%8),
			Quantity:   1 + float64(index%16),
		}
	}

	for b.Loop() {
		statistics := CoordinateStatistics{}
		_ = statistics.Measure(orders, 100, 1e-9)
	}
}
