package trader

import (
	"testing"

	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

/*
btcUSDChecksumSnapshot is the exact worked example from Kraken's own
"Book checksum (WebSocket v2)" guide. Its checksum, 3310070434, is
Kraken's documented expected result, giving a ground-truth check on the
algorithm independent of this codebase's decoding.
*/
const btcUSDChecksumSnapshot = `{"channel":"book","type":"snapshot","data":[{
	"symbol": "BTC/USD",
	"bids": [
		{"price": 45283.5, "qty": 0.10000000},
		{"price": 45283.4, "qty": 1.54582015},
		{"price": 45282.1, "qty": 0.10000000},
		{"price": 45281.0, "qty": 0.10000000},
		{"price": 45280.3, "qty": 1.54592586},
		{"price": 45279.0, "qty": 0.07990000},
		{"price": 45277.6, "qty": 0.03310103},
		{"price": 45277.5, "qty": 0.30000000},
		{"price": 45277.3, "qty": 1.54602737},
		{"price": 45276.6, "qty": 0.15445238}
	],
	"asks": [
		{"price": 45285.2, "qty": 0.00100000},
		{"price": 45286.4, "qty": 1.54571953},
		{"price": 45286.6, "qty": 1.54571109},
		{"price": 45289.6, "qty": 1.54560911},
		{"price": 45290.2, "qty": 0.15890660},
		{"price": 45291.8, "qty": 1.54553491},
		{"price": 45294.7, "qty": 0.04454749},
		{"price": 45296.1, "qty": 0.35380000},
		{"price": 45297.5, "qty": 0.09945542},
		{"price": 45299.5, "qty": 0.18772827}
	],
	"checksum": 3310070434,
	"timestamp": "2023-10-06T17:35:55.440295Z"
}]}`

func TestOrderBookApply(t *testing.T) {
	Convey("Given Kraken's own documented checksum worked example", t, func() {
		orderBook := NewOrderBook(10)
		rows := kraken.NewBookDataSlice([]byte(btcUSDChecksumSnapshot))
		So(rows, ShouldHaveLength, 1)

		Convey("When the snapshot is applied", func() {
			valid := orderBook.Apply(rows[0], 8)

			Convey("Then the reconstructed checksum matches Kraken's documented result", func() {
				So(valid, ShouldBeTrue)
				So(orderBook.Invalid("BTC/USD"), ShouldBeFalse)
			})

			Convey("Then the top of book is the best bid and ask", func() {
				bid, ask, ok := orderBook.TopOfBook("BTC/USD")
				So(ok, ShouldBeTrue)
				So(bid.String(), ShouldEqual, "45283.5")
				So(ask.String(), ShouldEqual, "45285.2")
			})
		})
	})

	Convey("Given a MATIC/USD snapshot with a checksum recomputed for qty_precision 8", t, func() {
		orderBook := NewOrderBook(25)
		rows := kraken.NewBookDataSlice([]byte(`{"channel":"book","type":"snapshot","data":[{"symbol":"MATIC/USD","bids":[{"price":0.5666,"qty":4831.75496356},{"price":0.5665,"qty":6658.22734739},{"price":0.5664,"qty":18724.91513344},{"price":0.5663,"qty":11563.92544914},{"price":0.5662,"qty":14006.65365711},{"price":0.5661,"qty":17454.85679807},{"price":0.566,"qty":18097.1547},{"price":0.5659,"qty":33644.89175666},{"price":0.5658,"qty":148.3464},{"price":0.5657,"qty":606.70854372}],"asks":[{"price":0.5668,"qty":4410.79769741},{"price":0.5669,"qty":4655.40412487},{"price":0.567,"qty":49844.89424998},{"price":0.5671,"qty":24306.41678},{"price":0.5672,"qty":29783.25223475},{"price":0.5673,"qty":57234.71239278},{"price":0.5674,"qty":45065.04744},{"price":0.5675,"qty":5912.76380354},{"price":0.5676,"qty":42514.92434778},{"price":0.5677,"qty":36304.0847022}],"checksum":3187404416,"timestamp":"2023-10-06T17:35:55.440295Z"}]}`))
		So(rows, ShouldHaveLength, 1)

		Convey("When the snapshot is applied with the matching qty_precision", func() {
			valid := orderBook.Apply(rows[0], 8)

			Convey("Then the checksum validates", func() {
				So(valid, ShouldBeTrue)
			})
		})
	})

	Convey("Given a valid snapshot followed by a corrupting delta", t, func() {
		orderBook := NewOrderBook(10)
		rows := kraken.NewBookDataSlice([]byte(btcUSDChecksumSnapshot))
		So(orderBook.Apply(rows[0], 8), ShouldBeTrue)

		Convey("When an update carries a checksum that no longer matches the merged book", func() {
			update := kraken.NewBookDataSlice([]byte(`{"channel":"book","type":"update","data":[{
				"symbol": "BTC/USD",
				"bids": [{"price": 45283.5, "qty": 999}],
				"asks": [],
				"checksum": 1,
				"timestamp": "2023-10-06T17:35:55.440295Z"
			}]}`))
			valid := orderBook.Apply(update[0], 8)

			Convey("Then the book is marked invalid and top of book is withheld", func() {
				So(valid, ShouldBeFalse)
				So(orderBook.Invalid("BTC/USD"), ShouldBeTrue)
				_, _, ok := orderBook.TopOfBook("BTC/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given an update that removes a price level with qty zero", t, func() {
		orderBook := NewOrderBook(10)
		rows := kraken.NewBookDataSlice([]byte(btcUSDChecksumSnapshot))
		So(orderBook.Apply(rows[0], 8), ShouldBeTrue)

		Convey("When the top ask is pulled", func() {
			update := kraken.NewBookDataSlice([]byte(`{"channel":"book","type":"update","data":[{
				"symbol": "BTC/USD",
				"bids": [],
				"asks": [{"price": 45285.2, "qty": 0}],
				"checksum": 1182027464,
				"timestamp": "2023-10-06T17:35:55.440295Z"
			}]}`))
			valid := orderBook.Apply(update[0], 8)

			Convey("Then the next best ask becomes the top of book", func() {
				So(valid, ShouldBeTrue)
				_, ask, ok := orderBook.TopOfBook("BTC/USD")
				So(ok, ShouldBeTrue)
				So(ask.String(), ShouldEqual, "45286.4")
			})
		})
	})

	Convey("Given a snapshot deeper than the retained depth", t, func() {
		orderBook := NewOrderBook(3)
		rows := kraken.NewBookDataSlice([]byte(`{"channel":"book","type":"snapshot","data":[{"symbol":"ETH/USD","bids":[{"price":100,"qty":1},{"price":99,"qty":1},{"price":98,"qty":1},{"price":97,"qty":1}],"asks":[{"price":101,"qty":1},{"price":102,"qty":1},{"price":103,"qty":1},{"price":104,"qty":1}],"checksum":1,"timestamp":"2023-10-06T17:35:55.440295Z"}]}`))

		Convey("When it is applied", func() {
			orderBook.Apply(rows[0], 8)
			book := orderBook.book("ETH/USD")

			Convey("Then only the configured depth is retained per side", func() {
				So(len(book.bids), ShouldEqual, 3)
				So(len(book.asks), ShouldEqual, 3)
			})
		})
	})
}

func BenchmarkOrderBookApply(b *testing.B) {
	rows := kraken.NewBookDataSlice([]byte(btcUSDChecksumSnapshot))
	orderBook := NewOrderBook(10)

	b.ReportAllocs()
	for b.Loop() {
		orderBook.Apply(rows[0], 8)
	}
}
