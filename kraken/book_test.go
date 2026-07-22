package kraken

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewBookDataSlice(t *testing.T) {
	Convey("Given Kraken book data payloads", t, func() {
		payload := []byte(`[{
			"symbol": "MATIC/USD",
			"bids": [{"price": 0.5666, "qty": 4831.75496356}],
			"asks": [{"price": 0.5668, "qty": 4410.79769741}],
			"checksum": 2439117997,
			"timestamp": "2023-10-06T17:35:55.440295Z"
		}]`)

		books := NewBookDataSlice(payload)

		Convey("It should decode book levels and checksum", func() {
			So(len(books), ShouldEqual, 1)

			book := books[0]

			So(book.Symbol, ShouldEqual, "MATIC/USD")
			So(book.Checksum, ShouldEqual, uint32(2439117997))
			So(book.Bids[0].Price.String(), ShouldEqual, "0.5666")
			So(book.Asks[0].Qty, ShouldAlmostEqual, 4410.79769741)
			So(book.Timestamp.IsZero(), ShouldBeFalse)
		})
	})

	Convey("Given a book frame with envelope type", t, func() {
		payload := []byte(`{"type":"update","data":[{"symbol":"MATIC/USD","bids":[{"price":0.5657,"qty":1098.3947558}],"asks":[],"checksum":2114181697,"timestamp":"2023-10-06T17:35:55.440295Z"}]}`)

		books := NewBookDataSlice(payload)

		Convey("It should copy the envelope type onto each row", func() {
			So(len(books), ShouldEqual, 1)
			So(books[0].Type, ShouldEqual, "update")
		})
	})
}

/*
TestPriceTick proves exact exchange-lattice conversion rejects nearby decimal
prices instead of silently rounding them onto a valid tick.
*/
func TestPriceTick(t *testing.T) {
	increment, err := decimal.NewFromString("0.01")

	if err != nil {
		t.Fatal(err)
	}

	Convey("Given Kraken's declared price increment", t, func() {
		price, err := decimal.NewFromString("100.02")
		So(err, ShouldBeNil)

		Convey("An exact lattice price should resolve to its tick", func() {
			tick, err := PriceTick(*price, *increment)
			So(err, ShouldBeNil)
			So(tick, ShouldEqual, int64(10002))
		})

		Convey("A nearby off-lattice price should be rejected", func() {
			price, err := decimal.NewFromString("100.025")
			So(err, ShouldBeNil)
			_, err = PriceTick(*price, *increment)
			So(err, ShouldNotBeNil)
		})
	})
}

/*
BenchmarkPriceTick measures exact exchange-lattice conversion.
*/
func BenchmarkPriceTick(b *testing.B) {
	price, err := decimal.NewFromString("100.02")

	if err != nil {
		b.Fatal(err)
	}

	increment, err := decimal.NewFromString("0.01")

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := PriceTick(*price, *increment); err != nil {
			b.Fatal(err)
		}
	}
}
