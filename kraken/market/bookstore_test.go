package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBookStoreApplySnapshot(t *testing.T) {
	Convey("Given a book snapshot", t, func() {
		store := NewBookStore(10)
		update := &BookUpdate{
			Symbol: "BTC/USD",
			Type:   "snapshot",
			Bids:   []BookLevel{{Price: 100, Qty: 1}},
			Asks:   []BookLevel{{Price: 101, Qty: 2}},
			Checksum: int64(checksumBook(&BookUpdate{
				Symbol: "BTC/USD",
				Bids:   []BookLevel{{Price: 100, Qty: 1}},
				Asks:   []BookLevel{{Price: 101, Qty: 2}},
			}, 10)),
		}

		Convey("It should store the book without checksum error", func() {
			So(store.Apply(update), ShouldBeNil)

			snapshot, ok := store.Snapshot("BTC/USD")

			So(ok, ShouldBeTrue)
			So(snapshot.Asks[0].Price, ShouldEqual, 101)
		})
	})
}

func TestBookStoreVWAP(t *testing.T) {
	Convey("Given depth on both sides", t, func() {
		store := NewBookStore(10)
		update := &BookUpdate{
			Symbol: "BTC/USD",
			Type:   "snapshot",
			Asks: []BookLevel{
				{Price: 100, Qty: 1},
				{Price: 102, Qty: 1},
			},
			Bids: []BookLevel{{Price: 99, Qty: 1}},
		}

		So(store.Apply(update), ShouldBeNil)

		Convey("It should compute buy-side VWAP through asks", func() {
			price, filled, err := store.VWAP("BTC/USD", "buy", 1.5)

			So(err, ShouldBeNil)
			So(filled, ShouldEqual, 1.5)
			So(price, ShouldAlmostEqual, 100.6666666667, 1e-6)
		})
	})
}

func TestInstrumentRegistryObserve(t *testing.T) {
	Convey("Given an instrument update", t, func() {
		registry := NewInstrumentRegistry()

		registry.Observe(&InstrumentUpdate{
			Pairs: []InstrumentPair{{
				Symbol:         "BTC/USD",
				Status:         "online",
				QtyIncrement:   0.0001,
				QtyMin:         0.0001,
				CostMin:        10,
				PriceIncrement: 0.1,
			}},
		})

		Convey("It should expose constraints for online symbols", func() {
			constraints, ok := registry.Constraints("BTC/USD")

			So(ok, ShouldBeTrue)
			So(constraints.QtyIncrement, ShouldEqual, 0.0001)
		})
	})
}
