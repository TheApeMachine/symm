package manifold

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFramesPlace(t *testing.T) {
	Convey("Given a resident frame fed one symbol's book", t, func() {
		registry := newFrames()

		for _, price := range []float64{100, 101, 99, 102, 98} {
			registry.place("BTC/USD", math.Log(price), math.Log(5))
		}

		Convey("Every placement lands inside the domain", func() {
			// tanh keeps a far-out order on the axis instead of wrapping it
			// onto the opposite face, which is what an unbounded z-score does
			// in a periodic domain.
			for _, price := range []float64{1, 50, 100, 200, 100000} {
				x, y, _, _ := registry.place("BTC/USD", math.Log(price), math.Log(price))

				So(x, ShouldBeGreaterThanOrEqualTo, 0)
				So(x, ShouldBeLessThanOrEqualTo, 1)
				So(y, ShouldBeGreaterThanOrEqualTo, 0)
				So(y, ShouldBeLessThanOrEqualTo, 1)
			}
		})

		Convey("A price above the frame's centre sits above the middle", func() {
			high, _, _, _ := registry.place("BTC/USD", math.Log(120), math.Log(5))

			So(high, ShouldBeGreaterThan, 0.5)
		})

		Convey("A price below the frame's centre sits below the middle", func() {
			low, _, _, _ := registry.place("BTC/USD", math.Log(80), math.Log(5))

			So(low, ShouldBeLessThan, 0.5)
		})

		Convey("Distinct prices receive distinct coordinates", func() {
			first, _, _, _ := registry.place("BTC/USD", math.Log(99.5), math.Log(5))
			second, _, _, _ := registry.place("BTC/USD", math.Log(100.5), math.Log(5))

			So(first, ShouldNotAlmostEqual, second, 1e-9)
		})

		Convey("Each symbol carries its own frame", func() {
			registry.place("DOGE/USD", math.Log(0.1), math.Log(1000))
			far, _, _, _ := registry.place("DOGE/USD", math.Log(0.1), math.Log(1000))

			So(far, ShouldBeGreaterThan, 0)
			So(far, ShouldBeLessThan, 1)
		})
	})

	Convey("Given a frame that has seen nothing", t, func() {
		registry := newFrames()

		Convey("The first order still places, at the centre it defines", func() {
			x, y, _, _ := registry.place("BTC/USD", math.Log(100), math.Log(5))

			So(x, ShouldAlmostEqual, 0.5, 1e-9)
			So(y, ShouldAlmostEqual, 0.5, 1e-9)
		})
	})
}

func TestFramesPlacePrice(t *testing.T) {
	Convey("Given a frame that has seen a symbol's book", t, func() {
		registry := newFrames()

		for _, price := range []float64{100, 101, 99, 102, 98} {
			registry.place("BTC/USD", math.Log(price), math.Log(5))
		}

		Convey("A probe is placed on the price axis by the same frame", func() {
			x, _ := registry.placePrice("BTC/USD", math.Log(120))

			So(x, ShouldBeGreaterThan, 0.5)
			So(x, ShouldBeLessThan, 1)
		})
	})
}

func TestQueueDepth(t *testing.T) {
	Convey("Given orders in a stated queue", t, func() {
		Convey("Depth sweeps the axis front to back", func() {
			So(queueDepth(0, 4, 0), ShouldEqual, 0)
			So(queueDepth(3, 4, 0), ShouldEqual, 1)
		})
	})

	Convey("Given an update stating no queue", t, func() {
		Convey("Distinct orders do not collapse onto one coordinate", func() {
			So(queueDepth(0, 1, 12345), ShouldNotEqual, queueDepth(0, 1, 54321))
		})

		Convey("And every order stays on the axis", func() {
			So(queueDepth(0, 1, 0xFFFFFFFF), ShouldBeLessThanOrEqualTo, 1)
			So(queueDepth(0, 1, 0), ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}
