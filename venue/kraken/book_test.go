package kraken

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSide(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When Side constants are inspected", func() {
			Convey("bid and ask match the wire labels", func() {
				So(SideBid, ShouldEqual, Side("bid"))
				So(SideAsk, ShouldEqual, Side("ask"))
			})
		})
	})
}

func TestBook(t *testing.T) {
	Convey("Setup", t, func() {
		raw := []byte(`{
			"channel":"book",
			"type":"snapshot",
			"data":[{
				"symbol":"PIPE/USD",
				"type":"snapshot",
				"bids":[{"price":"1.10","qty":2}],
				"asks":[{"price":"1.20","qty":3}],
				"checksum":42,
				"timestamp":"2024-01-02T03:04:05.000Z"
			}]
		}`)

		Convey("When Book JSON is unmarshaled", func() {
			var book Book
			err := json.Unmarshal(raw, &book)

			Convey("Fields match the wire payload", func() {
				So(err, ShouldBeNil)
				So(book.Channel, ShouldEqual, "book")
				So(book.Type, ShouldEqual, "snapshot")
				So(len(book.Data), ShouldEqual, 1)
				So(book.Data[0].Symbol, ShouldEqual, "PIPE/USD")
				So(book.Data[0].Checksum, ShouldEqual, uint32(42))
				So(book.Data[0].Bids[0].Price.String(), ShouldEqual, "1.1")
				So(book.Data[0].Bids[0].Qty, ShouldEqual, 2.0)
				So(book.Data[0].Asks[0].Price.String(), ShouldEqual, "1.2")
				So(book.Data[0].Asks[0].Qty, ShouldEqual, 3.0)
			})
		})
	})
}

func BenchmarkSide(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = SideBid
		_ = SideAsk
	}
}

func BenchmarkBook(b *testing.B) {
	raw := []byte(`{
		"channel":"book",
		"type":"snapshot",
		"data":[{
			"symbol":"PIPE/USD",
			"type":"snapshot",
			"bids":[{"price":"1.10","qty":2}],
			"asks":[{"price":"1.20","qty":3}],
			"checksum":42,
			"timestamp":"2024-01-02T03:04:05.000Z"
		}]
	}`)

	b.ReportAllocs()

	for b.Loop() {
		var book Book
		_ = json.Unmarshal(raw, &book)
	}
}
