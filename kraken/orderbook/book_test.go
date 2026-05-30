package orderbook

import (
	"hash/crc32"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func level(price, qty float64, priceRaw, qtyRaw string) Level {
	return Level{Price: price, Qty: qty, PriceRaw: priceRaw, QtyRaw: qtyRaw}
}

func TestBookApplySnapshot(t *testing.T) {
	convey.Convey("Given an empty book at depth 2", t, func() {
		book := NewBook(2)

		convey.Convey("When a snapshot of unsorted, over-deep levels is applied", func() {
			book.ApplySnapshot(
				[]Level{
					level(99.0, 1, "99.0", "1"),
					level(101.0, 2, "101.0", "2"),
					level(100.0, 3, "100.0", "3"),
				},
				[]Level{
					level(103.0, 1, "103.0", "1"),
					level(102.0, 2, "102.0", "2"),
					level(104.0, 3, "104.0", "3"),
				},
			)

			convey.Convey("Bids are sorted best-first and trimmed to depth", func() {
				bids := book.Bids()
				convey.So(len(bids), convey.ShouldEqual, 2)
				convey.So(bids[0].Price, convey.ShouldEqual, 101.0)
				convey.So(bids[1].Price, convey.ShouldEqual, 100.0)
			})

			convey.Convey("Asks are sorted best-first and trimmed to depth", func() {
				asks := book.Asks()
				convey.So(len(asks), convey.ShouldEqual, 2)
				convey.So(asks[0].Price, convey.ShouldEqual, 102.0)
				convey.So(asks[1].Price, convey.ShouldEqual, 103.0)
			})

			convey.Convey("The book is ready", func() {
				convey.So(book.Ready(), convey.ShouldBeTrue)
			})
		})
	})
}

func TestBookApplyDelta(t *testing.T) {
	convey.Convey("Given a book seeded with a snapshot", t, func() {
		book := NewBook(0)
		book.ApplySnapshot(
			[]Level{
				level(100.0, 5, "100.0", "5"),
				level(99.0, 4, "99.0", "4"),
			},
			[]Level{
				level(101.0, 5, "101.0", "5"),
				level(102.0, 4, "102.0", "4"),
			},
		)

		convey.Convey("When a delta updates one level, adds another, and removes a third", func() {
			book.ApplyDelta(
				[]Level{
					level(100.0, 8, "100.0", "8"), // update existing
					level(98.0, 2, "98.0", "2"),   // insert new
					level(99.0, 0, "99.0", "0"),   // remove (qty 0)
				},
				nil,
			)

			convey.Convey("The untouched levels survive and the delta is folded in", func() {
				bids := book.Bids()
				convey.So(len(bids), convey.ShouldEqual, 2)
				convey.So(bids[0].Price, convey.ShouldEqual, 100.0)
				convey.So(bids[0].Qty, convey.ShouldEqual, 8.0)
				convey.So(bids[1].Price, convey.ShouldEqual, 98.0)
			})

			convey.Convey("The side not mentioned in the delta is left intact", func() {
				convey.So(len(book.Asks()), convey.ShouldEqual, 2)
			})
		})

		convey.Convey("When a delta removes a price that is not in the book", func() {
			book.ApplyDelta([]Level{level(50.0, 0, "50.0", "0")}, nil)

			convey.Convey("The book is unchanged", func() {
				convey.So(len(book.Bids()), convey.ShouldEqual, 2)
			})
		})
	})
}

func TestNormalizeChecksumToken(t *testing.T) {
	convey.Convey("Given Kraken raw numeric tokens", t, func() {
		convey.Convey("The decimal point and leading zeros are stripped", func() {
			convey.So(normalizeChecksumToken("0.00010000"), convey.ShouldEqual, "10000")
			convey.So(normalizeChecksumToken("5.00000000"), convey.ShouldEqual, "500000000")
			convey.So(normalizeChecksumToken("1234.5"), convey.ShouldEqual, "12345")
		})
	})
}

func TestBookChecksum(t *testing.T) {
	convey.Convey("Given a book built from raw exchange tokens", t, func() {
		book := NewBook(0)
		book.ApplySnapshot(
			[]Level{
				level(0.00009500, 10, "0.00009500", "10.00000000"),
				level(0.00009000, 3, "0.00009000", "3.00000000"),
			},
			[]Level{
				level(0.00010000, 5, "0.00010000", "5.00000000"),
				level(0.00010500, 2.5, "0.00010500", "2.50000000"),
			},
		)

		convey.Convey("The checksum matches an independently built CRC32 string", func() {
			// Asks (best first) then bids (best first); each level price then qty,
			// decimal point and leading zeros stripped, trailing zeros kept.
			expected := crc32.ChecksumIEEE([]byte(
				"10000" + "500000000" + // ask 0.00010000 / 5.00000000
					"10500" + "250000000" + // ask 0.00010500 / 2.50000000
					"9500" + "1000000000" + // bid 0.00009500 / 10.00000000
					"9000" + "300000000", // bid 0.00009000 / 3.00000000
			))

			convey.So(book.Checksum(), convey.ShouldEqual, expected)
		})

		convey.Convey("Verify accepts the matching checksum and rejects a wrong one", func() {
			convey.So(book.Verify(book.Checksum()), convey.ShouldBeTrue)
			convey.So(book.Verify(book.Checksum()+1), convey.ShouldBeFalse)
		})
	})
}

func BenchmarkBookApplyDelta(b *testing.B) {
	book := NewBook(10)
	book.ApplySnapshot(
		[]Level{level(100.0, 5, "100.0", "5"), level(99.0, 4, "99.0", "4")},
		[]Level{level(101.0, 5, "101.0", "5"), level(102.0, 4, "102.0", "4")},
	)

	delta := []Level{level(100.0, 7, "100.0", "7")}

	for b.Loop() {
		book.ApplyDelta(delta, nil)
	}
}
