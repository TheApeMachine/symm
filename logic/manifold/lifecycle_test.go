package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestApply(t *testing.T) {
	Convey("Given an empty resident order lifecycle", t, func() {
		lifecycle := newOrderLifecycle()
		add := kraken.Level3Data{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event:      "add",
				OrderID:    "bid-1",
				LimitPrice: decimalPtr(100),
				OrderQty:   decimalPtr(2),
			}},
		}
		departures, err := lifecycle.Apply(add)
		So(err, ShouldBeNil)
		So(departures, ShouldBeEmpty)
		contentID := orderContentID(orderIdentity{symbol: "BTC/USD", orderID: "bid-1"})

		Convey("A named delete returns the exact resident ContentID", func() {
			departures, err := lifecycle.Apply(kraken.Level3Data{
				Symbol: "BTC/USD",
				Bids: []kraken.Level3Order{{
					Event:   "delete",
					OrderID: "bid-1",
				}},
			})

			So(err, ShouldBeNil)
			So(departures, ShouldResemble, []int64{contentID})
			So(lifecycle.byContent, ShouldBeEmpty)
		})

		Convey("A replacement snapshot departs the prior symbol population", func() {
			departures, err := lifecycle.Apply(kraken.Level3Data{
				Symbol: "BTC/USD",
				Type:   "snapshot",
				Asks: []kraken.Level3Order{{
					OrderID:    "ask-1",
					LimitPrice: decimalPtr(101),
					OrderQty:   decimalPtr(3),
				}},
			})

			So(err, ShouldBeNil)
			So(departures, ShouldResemble, []int64{contentID})
			So(lifecycle.byContent, ShouldHaveLength, 1)
		})
	})

	Convey("Given a delete for an identity never made resident", t, func() {
		lifecycle := newOrderLifecycle()
		departures, err := lifecycle.Apply(kraken.Level3Data{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event:   "delete",
				OrderID: "missing",
			}},
		})

		So(err, ShouldNotBeNil)
		So(departures, ShouldBeNil)
	})
}

func TestOrderContentID(t *testing.T) {
	Convey("Content identity includes both symbol and venue order ID", t, func() {
		first := orderContentID(orderIdentity{symbol: "BTC/USD", orderID: "same"})
		second := orderContentID(orderIdentity{symbol: "ETH/USD", orderID: "same"})

		So(first, ShouldNotEqual, second)
		So(first, ShouldEqual, orderContentID(orderIdentity{
			symbol:  "BTC/USD",
			orderID: "same",
		}))
	})
}

func BenchmarkApply(b *testing.B) {
	lifecycle := newOrderLifecycle()
	add := kraken.Level3Data{
		Symbol: "BTC/USD",
		Bids: []kraken.Level3Order{{
			Event:      "add",
			OrderID:    "bid-1",
			LimitPrice: decimalPtr(100),
			OrderQty:   decimalPtr(2),
		}},
	}
	remove := kraken.Level3Data{
		Symbol: "BTC/USD",
		Bids: []kraken.Level3Order{{
			Event:   "delete",
			OrderID: "bid-1",
		}},
	}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := lifecycle.Apply(add); err != nil {
			b.Fatal(err)
		}

		if _, err := lifecycle.Apply(remove); err != nil {
			b.Fatal(err)
		}
	}
}
