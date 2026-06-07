package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestDeriveBidAsk(t *testing.T) {
	Convey("Given explicit bid and ask", t, func() {
		bid, ask, ok := DeriveBidAsk(types.Measurement{
			Last: 100,
			Bid:  99,
			Ask:  101,
		})

		Convey("It should return the tape values", func() {
			So(ok, ShouldBeTrue)
			So(bid, ShouldEqual, 99)
			So(ask, ShouldEqual, 101)
		})
	})

	Convey("Given last and spread bps without bid/ask", t, func() {
		bid, ask, ok := DeriveBidAsk(types.Measurement{
			Last:      100,
			SpreadBPS: 200,
		})

		Convey("It should derive symmetric quotes from spread", func() {
			So(ok, ShouldBeTrue)
			So(bid, ShouldAlmostEqual, 99, 1e-9)
			So(ask, ShouldAlmostEqual, 101, 1e-9)
		})
	})

	Convey("Given last without spread or bid/ask", t, func() {
		_, _, ok := DeriveBidAsk(types.Measurement{Last: 100})

		Convey("It should refuse to invent a quote", func() {
			So(ok, ShouldBeFalse)
		})
	})
}

func TestApplyDerivedBidAsk(t *testing.T) {
	Convey("Given a measurement with spread but no book", t, func() {
		measurement := ApplyDerivedBidAsk(types.Measurement{
			Symbol:    "FXS/EUR",
			Last:      4.36,
			SpreadBPS: 800,
		})

		Convey("It should attach bid/ask without fabricating depth", func() {
			So(measurement.Bid, ShouldBeGreaterThan, 0)
			So(measurement.Ask, ShouldBeGreaterThan, measurement.Bid)
			So(measurement.HasBookDepth(), ShouldBeFalse)
		})
	})
}
