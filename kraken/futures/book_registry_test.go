package futures

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestBookRegistryApplySnapshot(t *testing.T) {
	convey.Convey("Given a futures book snapshot", t, func() {
		registry := NewBookRegistry()

		update := registry.ApplySnapshot(BookSnapshot{
			Feed:      FeedBookSnapshot,
			ProductID: "PI_XBTUSD",
			Timestamp: 1_612_269_825_817,
			Seq:       10,
			Bids: []krakenmarket.BookLevel{
				{Price: 100, Qty: 5},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 101, Qty: 4},
			},
		})

		convey.Convey("It should publish a product-scoped book update", func() {
			convey.So(update, convey.ShouldNotBeNil)
			convey.So(update.Symbol, convey.ShouldEqual, "PI_XBTUSD")
			convey.So(update.Type, convey.ShouldEqual, "snapshot")
			convey.So(len(update.Bids), convey.ShouldEqual, 1)
			convey.So(len(update.Asks), convey.ShouldEqual, 1)
		})
	})
}

func TestBookRegistryApplyDelta(t *testing.T) {
	convey.Convey("Given a snapshot followed by a sell-side delta", t, func() {
		registry := NewBookRegistry()

		registry.ApplySnapshot(BookSnapshot{
			ProductID: "PI_XBTUSD",
			Timestamp: 1_612_269_825_817,
			Seq:       10,
			Bids:      []krakenmarket.BookLevel{{Price: 100, Qty: 5}},
			Asks:      []krakenmarket.BookLevel{{Price: 101, Qty: 4}},
		})

		update, ok := registry.ApplyDelta(BookDelta{
			Feed:      FeedBookDelta,
			ProductID: "PI_XBTUSD",
			Side:      "sell",
			Seq:       11,
			Price:     101,
			Qty:       0,
			Timestamp: 1_612_269_825_900,
		})

		convey.Convey("It should remove depleted ask levels", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(update, convey.ShouldNotBeNil)
			convey.So(len(update.Asks), convey.ShouldEqual, 0)
		})
	})
}
