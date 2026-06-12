package signal

import (
	"container/ring"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestResolvedChange(t *testing.T) {
	Convey("Given flat prices with non-zero tick spread", t, func() {
		move, magnitude, ok := ResolvedChange([]float64{100, 100.01, 100})

		Convey("It should derive magnitude from the tick series", func() {
			So(ok, ShouldBeTrue)
			So(magnitude, ShouldBeGreaterThan, 0)
			So(move, ShouldEqual, magnitude)
		})
	})

	Convey("Given a directional move", t, func() {
		move, magnitude, ok := ResolvedChange([]float64{100, 101})

		Convey("It should keep the anchor change", func() {
			So(ok, ShouldBeTrue)
			So(magnitude, ShouldEqual, 0.01)
			So(move, ShouldEqual, 0.01)
		})
	})
}

func TestObservationElapsedClampsZero(t *testing.T) {
	Convey("Given a ring sample at the same instant as observedAt", t, func() {
		at := time.Unix(100, 0)
		measurements := ring.New(4)
		measurements.Value = &krakenmarket.TradeUpdate{
			Symbol:    "BTC/USD",
			Price:     100,
			Qty:       1,
			Timestamp: at,
		}
		measurements = measurements.Next()

		elapsed, err := ObservationElapsed(measurements, at)

		Convey("It should clamp to the minimum observation window", func() {
			So(err, ShouldBeNil)
			So(elapsed, ShouldEqual, minimumObservationSeconds)
		})
	})
}
