package trader

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/order"
)

func TestCashDeltaBuyIncludesQuoteFee(t *testing.T) {
	convey.Convey("Given a buy fill with EUR fee", t, func() {
		delta := cashDeltaBuy(order.Fill{
			Qty:    1,
			Price:  100,
			Fee:    0.4,
			FeeCcy: "EUR",
		}, "EUR")

		convey.Convey("It should include fee in cash spent", func() {
			convey.So(delta, convey.ShouldEqual, 100.4)
		})
	})
}

func TestCashDeltaSellSubtractsQuoteFee(t *testing.T) {
	convey.Convey("Given a sell fill with EUR fee", t, func() {
		delta := cashDeltaSell(order.Fill{
			Qty:    1,
			Price:  100,
			Fee:    0.4,
			FeeCcy: "EUR",
		}, "EUR")

		convey.Convey("It should subtract fee from proceeds", func() {
			convey.So(delta, convey.ShouldEqual, 99.6)
		})
	})
}

func TestLiveSessionPendingEntry(t *testing.T) {
	convey.Convey("Given a tracked pending entry", t, func() {
		session := &liveSession{}
		session.trackEntry("c1", "BTC/EUR", orderIntent{kind: "entry"})

		convey.Convey("It should report pending", func() {
			convey.So(session.HasPendingEntry("BTC/EUR"), convey.ShouldBeTrue)
		})

		convey.Convey("It should clear after drop", func() {
			session.dropIntent("c1", "BTC/EUR")
			convey.So(session.HasPendingEntry("BTC/EUR"), convey.ShouldBeFalse)
		})
	})
}
