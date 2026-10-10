package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestShadowFill(t *testing.T) {
	Convey("Given a shadow ledger and an ask side of 1@101 and 2@102", t, func() {
		shadow := NewShadow()
		view := &BookView{
			At:   time.Unix(1_700_000_000, 0),
			Bids: []BookLevel{{Price: 100, Quantity: 4}},
			Asks: []BookLevel{{Price: 101, Quantity: 1}, {Price: 102, Quantity: 2}},
		}

		first := shadow.Fill("X/USD", BUY, view, 1.5)

		Convey("A fill walks the levels best first", func() {
			So(first.Defined, ShouldBeTrue)
			So(first.Quantity, ShouldEqual, 1.5)
			So(first.Gross, ShouldEqual, 101+0.5*102)
			So(first.Short, ShouldEqual, 0)
		})

		Convey("A later fill on the unrefreshed version sees only what the first left", func() {
			second := shadow.Fill("X/USD", BUY, view, 2)

			So(second.Quantity, ShouldEqual, 1.5)
			So(second.Gross, ShouldEqual, 1.5*102)
			So(second.Short, ShouldEqual, 0.5)
		})

		Convey("A level whose displayed quantity changed has refreshed and is whole again", func() {
			next := &BookView{
				At:   view.At.Add(time.Second),
				Asks: []BookLevel{{Price: 101, Quantity: 1}, {Price: 102, Quantity: 3}},
			}

			second := shadow.Fill("X/USD", BUY, next, 3.5)

			// 101 still shows the quantity it had when it was emptied: still
			// consumed. 102 changed from 2 to 3: refreshed, all 3 available.
			So(second.Quantity, ShouldEqual, 3)
			So(second.Gross, ShouldEqual, 3*102)
			So(second.Short, ShouldEqual, 0.5)
		})

		Convey("A quote does not consume, and the adjusted view shows the consumption", func() {
			quote := shadow.Quote("X/USD", BUY, view, 1.5)
			So(quote.Quantity, ShouldEqual, 1.5)
			So(shadow.Quote("X/USD", BUY, view, 1.5).Gross, ShouldEqual, quote.Gross)

			adjusted := shadow.Adjusted("X/USD", view)
			So(adjusted.Asks, ShouldResemble, []BookLevel{{Price: 102, Quantity: 1.5}})
			So(adjusted.Bids, ShouldResemble, view.Bids)
			So(view.Asks[0].Quantity, ShouldEqual, 1)
		})

		Convey("Sells walk the bids independently of buys", func() {
			sell := shadow.Fill("X/USD", SELL, view, 1)
			So(sell.Gross, ShouldEqual, 100)
		})

		Convey("No as-of book leaves the fill undefined", func() {
			missing := shadow.Fill("X/USD", BUY, nil, 1)
			So(missing.Defined, ShouldBeFalse)
			So(missing.Short, ShouldEqual, 1)
		})
	})
}
