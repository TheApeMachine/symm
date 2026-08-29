package pumpdump

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func pumpdumpOrder(price, qty float64, at time.Time) kraken.Level3Order {
	return kraken.Level3Order{
		Event:      "add",
		OrderID:    "order",
		LimitPrice: decimal.NewFromFloat64(price),
		OrderQty:   decimal.NewFromFloat64(qty),
		Timestamp:  at,
	}
}

func pumpdumpMessage(symbol string, at time.Time, bids, asks []kraken.Level3Order) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol:    symbol,
		Timestamp: at,
		Bids:      bids,
		Asks:      asks,
	}
}

func TestLevel3Step(t *testing.T) {
	Convey("Given a message with an executable touch", t, func() {
		entity := NewLevel3()
		at := time.Unix(1_700_000_000, 0)

		Convey("Step derives the touch from the message's own orders", func() {
			measurement := entity.Step(pumpdumpMessage("BTC/USD", at,
				[]kraken.Level3Order{pumpdumpOrder(99, 1, at)},
				[]kraken.Level3Order{pumpdumpOrder(101, 1, at)},
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["spread"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.02, 1e-12)

			// A stateless direct measurement is whole.
			So(measurement.Maturity, ShouldEqual, 1.0)
		})
	})

	Convey("Given a message with no usable touch on either side", t, func() {
		entity := NewLevel3()

		Convey("Step returns a descriptive measurement error for the missing touch", func() {
			measurement := entity.Step(pumpdumpMessage("MISSING", time.Unix(1_700_000_000, 0), nil, nil))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
