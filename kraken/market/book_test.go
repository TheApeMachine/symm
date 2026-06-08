package market

import (
	"strconv"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

// krakenBookChecksumSample is the BTC/USD snapshot from Kraken's WebSocket v2
// book checksum guide (expected CRC32: 3310070434).
// https://docs.kraken.com/api/docs/guides/spot-ws-book-v2/
const krakenBookChecksumSample = `{
	"symbol": "BTC/USD",
	"bids": [
		{"price": "45283.5", "qty": "0.10000000"},
		{"price": "45283.4", "qty": "1.54582015"},
		{"price": "45282.1", "qty": "0.10000000"},
		{"price": "45281.0", "qty": "0.10000000"},
		{"price": "45280.3", "qty": "1.54592586"},
		{"price": "45279.0", "qty": "0.07990000"},
		{"price": "45277.6", "qty": "0.03310103"},
		{"price": "45277.5", "qty": "0.30000000"},
		{"price": "45277.3", "qty": "1.54602737"},
		{"price": "45276.6", "qty": "0.15445238"}
	],
	"asks": [
		{"price": "45285.2", "qty": "0.00100000"},
		{"price": "45286.4", "qty": "1.54571953"},
		{"price": "45286.6", "qty": "1.54571109"},
		{"price": "45289.6", "qty": "1.54560911"},
		{"price": "45290.2", "qty": "0.15890660"},
		{"price": "45291.8", "qty": "1.54553491"},
		{"price": "45294.7", "qty": "0.04454749"},
		{"price": "45296.1", "qty": "0.35380000"},
		{"price": "45297.5", "qty": "0.09945542"},
		{"price": "45299.5", "qty": "0.18772827"}
	],
	"checksum": 3310070434
}`

func bookLevel(price, qty float64) BookLevel {
	return BookLevel{
		Price:    price,
		Qty:      qty,
		PriceRaw: strconv.FormatFloat(price, 'f', -1, 64),
		QtyRaw:   strconv.FormatFloat(qty, 'f', -1, 64),
	}
}

// krakenBookChecksumSampleNumeric is the same documented snapshot with price
// and qty as JSON NUMBERS — the form the live v2 wire actually sends. The
// checksum must survive numeric decoding with trailing zeros intact; this is
// the regression test for the FormatFloat round-trip that silently corrupted
// every live checksum.
const krakenBookChecksumSampleNumeric = `{
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
	"checksum": 3310070434
}`

func TestBookChecksumNumericWire(t *testing.T) {
	Convey("Given the documented sample in live numeric wire form", t, func() {
		var book Book

		err := sonic.Unmarshal([]byte(krakenBookChecksumSampleNumeric), &book)

		Convey("It should preserve trailing zeros and match the exchange checksum", func() {
			So(err, ShouldBeNil)
			So(book.Bids[0].QtyRaw, ShouldEqual, "0.10000000")
			So(book.ComputedChecksum(), ShouldEqual, int64(3310070434))
		})
	})
}

func TestBookFold(t *testing.T) {
	Convey("Given a book fed an update frame", t, func() {
		book := Book{}
		bids := []BookLevel{bookLevel(99, 8)}
		asks := []BookLevel{bookLevel(101, 4)}
		delta := Book{Bids: bids, Asks: asks}
		delta.SetEnvelopeType(BookUpdate)
		book.Fold(delta, 10)

		Convey("It should merge the delta levels", func() {
			So(len(book.Bids), ShouldEqual, 1)
			So(len(book.Asks), ShouldEqual, 1)
			So(book.Bids[0].Price, ShouldEqual, 99)
		})
	})

	Convey("Given Kraken's documented book checksum sample", t, func() {
		var book Book

		err := sonic.Unmarshal([]byte(krakenBookChecksumSample), &book)

		Convey("It should match the documented exchange checksum", func() {
			So(err, ShouldBeNil)
			So(book.ComputedChecksum(), ShouldEqual, int64(3310070434))
			So(book.Checksum, ShouldEqual, int64(3310070434))
		})
	})

	Convey("Given a book fed a checksum-valid snapshot", t, func() {
		book := Book{}
		bids := []BookLevel{bookLevel(99, 8)}
		asks := []BookLevel{bookLevel(101, 4)}
		snapshot := Book{
			Symbol: "ETH/EUR",
			Bids:   bids,
			Asks:   asks,
		}
		snapshot.SetEnvelopeType(BookSnapshot)
		snapshot.Checksum = snapshot.ComputedChecksum()
		book.Fold(snapshot, 10)

		Convey("It should retain the snapshot levels", func() {
			So(book.Bids[0].Price, ShouldEqual, 99)
			So(book.Asks[0].Price, ShouldEqual, 101)
		})

		Convey("It should preserve the exchange checksum", func() {
			So(book.ComputedChecksum(), ShouldEqual, snapshot.Checksum)
		})
	})
}

func TestBookTouchQuote(t *testing.T) {
	Convey("Given a folded book with both sides", t, func() {
		book := Book{
			Bids: []BookLevel{bookLevel(99, 8), bookLevel(98, 2)},
			Asks: []BookLevel{bookLevel(101, 4), bookLevel(102, 1)},
		}

		mid, spread, depth, ok := book.TouchQuote()

		Convey("It should return touch metrics", func() {
			So(ok, ShouldBeTrue)
			So(mid, ShouldEqual, 200)
			So(spread, ShouldEqual, 2)
			So(depth, ShouldEqual, 15)
		})
	})

	Convey("Given an empty ask side", t, func() {
		book := Book{Bids: []BookLevel{bookLevel(99, 8)}}

		_, _, _, ok := book.TouchQuote()

		Convey("It should report unavailable", func() {
			So(ok, ShouldBeFalse)
		})
	})
}

func BenchmarkBookFold(b *testing.B) {
	bids := []BookLevel{bookLevel(99, 8)}
	asks := []BookLevel{bookLevel(101, 4)}
	update := Book{Symbol: "ETH/EUR", Bids: bids, Asks: asks}
	update.SetEnvelopeType(BookSnapshot)

	b.ReportAllocs()

	for b.Loop() {
		book := Book{}
		book.Fold(update, 10)
	}
}
