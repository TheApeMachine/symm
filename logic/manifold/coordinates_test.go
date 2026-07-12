package manifold

import (
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

var benchmarkCoordinate Coordinate

func TestCoordinateMapperMapOrder(t *testing.T) {
	Convey("Given a mapper whose lifetime sample changes between epochs", t, func() {
		lifetime := NewLifetimeEstimator(4)
		lifetime.RecordCompleted(time.Second)
		lifetime.RecordCompleted(3 * time.Second)
		mapper := NewCoordinateMapper(time.Second, 1e-9, lifetime)
		at := time.Unix(10, 0)
		order := &PhysicalOrder{
			LimitPrice: 100,
			Quantity:   2,
			AddedAt:    at.Add(-time.Second),
		}
		transform := EpochTransform{PriceScale: 0.01, SizeScale: 1}
		first, firstReady := mapper.MapOrder(order, 100, at, transform)

		Convey("When the next epoch adds a completed lifetime", func() {
			lifetime.RecordCompleted(2 * time.Second)
			second, secondReady := mapper.MapOrder(order, 100, at, transform)

			Convey("It should map age from the refreshed exact CDF", func() {
				So(firstReady, ShouldBeTrue)
				So(secondReady, ShouldBeTrue)
				So(first.Age, ShouldEqual, 0.5)
				So(second.Age, ShouldAlmostEqual, 1.0/3.0)
			})
		})
	})
}

func BenchmarkCoordinateMapperMapOrder(b *testing.B) {
	const (
		lifetimeCapacity = 4096
		carrierCount     = 256
	)

	lifetime := NewLifetimeEstimator(lifetimeCapacity)

	for index := 0; index < lifetimeCapacity; index++ {
		duration := time.Duration(index+1) * time.Millisecond

		if index%4 == 0 {
			lifetime.Censor(duration)
			continue
		}

		lifetime.RecordCompleted(duration)
	}

	mapper := NewCoordinateMapper(time.Second, 1e-9, lifetime)
	baseAt := time.Unix(10, 0)
	orders := make([]*PhysicalOrder, carrierCount)

	for index := range orders {
		orders[index] = &PhysicalOrder{
			OrderID:    "order-" + strconv.Itoa(index),
			LimitPrice: 99 + float64(index%5),
			Quantity:   1 + float64(index%17),
			AddedAt:    baseAt.Add(-time.Duration(index+1) * time.Millisecond),
		}
	}

	b.ReportMetric(carrierCount, "carriers/epoch")
	b.ResetTimer()
	epoch := 0

	for b.Loop() {
		epoch++
		at := baseAt.Add(time.Duration(epoch) * time.Millisecond)
		lifetime.RecordCompleted(time.Duration(epoch%lifetimeCapacity+1) * time.Millisecond)
		transform, ready := mapper.BeginEpoch(orders, 100, at)

		if !ready {
			b.Fatal("coordinate transform not ready")
		}

		for _, order := range orders {
			coordinate, mapped := mapper.MapOrder(order, 100, at, transform)

			if !mapped {
				b.Fatal("order not mapped")
			}

			benchmarkCoordinate = coordinate
		}
	}
}
