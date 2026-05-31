package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuildPaperFillInvalidQuantity(t *testing.T) {
	Convey("Given a maker fee that consumes the full notional", t, func() {
		fill, err := (&Maker{
			Symbol:     "BTC/EUR",
			LimitPrice: 50000,
			Notional:   10,
			ClOrdID:    "maker-test",
			FeePct:     100,
		}).BuildPaperFill(MakerQueueContext{
			InitialQueueAheadBaseQty: 0,
			BidTradeVolume:           1,
		})

		Convey("It should reject before queue readiness", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "invalid maker quantity")
			So(fill.Qty, ShouldEqual, 0)
		})
	})
}
