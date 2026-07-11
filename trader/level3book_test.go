package trader

import (
	"testing"

	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

/*
btcUSDLevel3ChecksumSnapshot is the exact worked example from Kraken's
own "L3 checksum (WebSocket v2)" guide. Its checksum, 1063832831, is
Kraken's documented expected result, giving a ground-truth check on the
algorithm independent of this codebase's decoding. price_precision 1 and
qty_precision 8 match the field widths used throughout the guide's own
worked strings (e.g. "44939.4" and "0.88968699").
*/
const btcUSDLevel3ChecksumSnapshot = `{"channel": "level3", "type": "snapshot", "data": [{"symbol": "BTC/USD", "checksum": 1063832831, "bids": [{"order_id": "OTCFZG-YOE2Q-LQKNM3", "limit_price": 44939.4, "order_qty": 0.88968699, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OFGP5R-B3E7G-54EZD6", "limit_price": 44939.4, "order_qty": 0.4521, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OMPHVY-IZPJ4-KOKA3P", "limit_price": 44939.4, "order_qty": 0.1, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OAI5QZ-AMPLW-NBNO72", "limit_price": 44939.4, "order_qty": 0.14296323, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "O7VFZI-CTFWH-FF6EIR", "limit_price": 44939.4, "order_qty": 0.25, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "O472V3-ZG4EZ-OLD66C", "limit_price": 44939.4, "order_qty": 0.10292988, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OEK26P-BGPUK-LDHMD2", "limit_price": 44939.4, "order_qty": 0.3388, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OSMYPE-S5VOC-YSS3WM", "limit_price": 44939.4, "order_qty": 1.2814086, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OJPMIN-NXZL5-SOWP6V", "limit_price": 44937.1, "order_qty": 0.03346877, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "O6PUGE-SQWYQ-TRJEEE", "limit_price": 44934.7, "order_qty": 0.3563, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OPUOGC-Q532V-3OKLPM", "limit_price": 44930.2, "order_qty": 0.22734299, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OCIU7J-VB3CI-HPULSF", "limit_price": 44930.2, "order_qty": 0.01, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "ORWVAF-LJFLY-ZWEHDQ", "limit_price": 44930.2, "order_qty": 0.0555, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OYRAHE-PI5AN-7KOQ4E", "limit_price": 44930.2, "order_qty": 0.7, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OGBHYU-UILDD-6DLLYJ", "limit_price": 44930.2, "order_qty": 0.15, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "O74ZBU-K2TKC-R76XSW", "limit_price": 44928.0, "order_qty": 0.0010524, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OQVTQF-Y56MR-BM6LWL", "limit_price": 44919.6, "order_qty": 0.3387, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OYEH6U-ZCHA2-3HFR3W", "limit_price": 44919.5, "order_qty": 0.0761, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OLGPG7-HVKXU-J6SANK", "limit_price": 44912.0, "order_qty": 0.3563, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OHGC3L-FRZQ3-UIVZRU", "limit_price": 44909.7, "order_qty": 0.0669, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "O73C6Y-VZXYA-H4LDFY", "limit_price": 44901.9, "order_qty": 0.00088982, "timestamp": "2024-01-08T12:26:39.526146327Z"}], "asks": [{"order_id": "OFVLAA-HRSSP-BK75KB", "limit_price": 44939.5, "order_qty": 4.52308393, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OYBAMK-O5DKX-WMPUTM", "limit_price": 44939.5, "order_qty": 0.00111261, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "O3DRCT-J5M2S-KYV526", "limit_price": 44939.5, "order_qty": 0.001, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OF3X3A-72WZY-6EKA5F", "limit_price": 44939.5, "order_qty": 0.01, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OF5UA6-6IIZ2-YGQTSJ", "limit_price": 44950.0, "order_qty": 0.10334926, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OSDOZX-7UZ6Y-QDNPVI", "limit_price": 44953.0, "order_qty": 0.00064537, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OV7KTS-A2TWV-3XKRIA", "limit_price": 44955.0, "order_qty": 0.0025, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OOF2V5-RYOHC-GLRNPM", "limit_price": 44959.6, "order_qty": 0.3563, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OTVOVS-QLST3-3JG7JI", "limit_price": 44959.6, "order_qty": 0.3563, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OGZCIU-RDQ77-DAAL3P", "limit_price": 44960.1, "order_qty": 0.00338072, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OVLG3E-HYBQM-CWNGCY", "limit_price": 44960.2, "order_qty": 0.88967575, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OWEOFO-HUCJC-T37MVO", "limit_price": 44967.0, "order_qty": 3.14392283, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OVYTHY-D2N76-5QYREQ", "limit_price": 44978.5, "order_qty": 0.0677896, "timestamp": "2024-01-08T12:26:39.526146327Z"}, {"order_id": "OFO525-PHRVS-236RMN", "limit_price": 44979.2, "order_qty": 0.3563, "timestamp": "2024-01-08T12:26:39.526146327Z"}]}]}`

func TestLevel3BookApply(t *testing.T) {
	Convey("Given Kraken's own documented level3 checksum worked example", t, func() {
		level3Book := NewLevel3Book(10)
		rows := kraken.NewLevel3DataSlice([]byte(btcUSDLevel3ChecksumSnapshot))
		So(rows, ShouldHaveLength, 1)

		Convey("When the snapshot is applied", func() {
			valid := level3Book.Apply(rows[0], 1, 8)

			Convey("Then the reconstructed checksum matches Kraken's documented result", func() {
				So(valid, ShouldBeTrue)
				So(level3Book.Invalid("BTC/USD"), ShouldBeFalse)
			})

			Convey("Then the top of book is the best bid and ask", func() {
				bid, ask, ok := level3Book.TopOfBook("BTC/USD")
				So(ok, ShouldBeTrue)
				So(bid, ShouldAlmostEqual, 44939.4)
				So(ask, ShouldAlmostEqual, 44939.5)
			})
		})
	})

	Convey("Given a valid snapshot followed by a corrupting update", t, func() {
		level3Book := NewLevel3Book(10)
		rows := kraken.NewLevel3DataSlice([]byte(btcUSDLevel3ChecksumSnapshot))
		So(level3Book.Apply(rows[0], 1, 8), ShouldBeTrue)

		Convey("When an update carries a checksum that no longer matches the merged book", func() {
			update := kraken.NewLevel3DataSlice([]byte(`[{"symbol":"BTC/USD","type":"update","checksum":1,"bids":[{"event":"add","order_id":"NEW-ORDER","limit_price":44939.4,"order_qty":1,"timestamp":"2024-01-08T12:26:50Z"}],"asks":[]}]`))
			valid := level3Book.Apply(update[0], 1, 8)

			Convey("Then the book is marked invalid and top of book is withheld", func() {
				So(valid, ShouldBeFalse)
				So(level3Book.Invalid("BTC/USD"), ShouldBeTrue)
				_, _, ok := level3Book.TopOfBook("BTC/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given a valid snapshot and an update that deletes every order at the best bid", t, func() {
		level3Book := NewLevel3Book(10)
		rows := kraken.NewLevel3DataSlice([]byte(btcUSDLevel3ChecksumSnapshot))
		So(level3Book.Apply(rows[0], 1, 8), ShouldBeTrue)

		Convey("When all eight 44939.4 bids are deleted", func() {
			deletedOrderIDs := []string{
				"OTCFZG-YOE2Q-LQKNM3", "OFGP5R-B3E7G-54EZD6", "OMPHVY-IZPJ4-KOKA3P",
				"OAI5QZ-AMPLW-NBNO72", "O7VFZI-CTFWH-FF6EIR", "O472V3-ZG4EZ-OLD66C",
				"OEK26P-BGPUK-LDHMD2", "OSMYPE-S5VOC-YSS3WM",
			}
			bids := make([]kraken.Level3Order, 0, len(deletedOrderIDs))

			for _, orderID := range deletedOrderIDs {
				bids = append(bids, kraken.Level3Order{
					Event: "delete", OrderID: orderID, LimitPrice: 44939.4,
				})
			}

			update := kraken.Level3Data{
				Symbol:   "BTC/USD",
				Type:     "update",
				Checksum: 3045255171,
				Bids:     bids,
			}
			valid := level3Book.Apply(update, 1, 8)

			Convey("Then the checksum validates and the next best bid becomes the top of book", func() {
				So(valid, ShouldBeTrue)
				bid, _, ok := level3Book.TopOfBook("BTC/USD")
				So(ok, ShouldBeTrue)
				So(bid, ShouldAlmostEqual, 44937.1)
			})
		})
	})

	Convey("Given a valid snapshot and a modify on an existing order", t, func() {
		level3Book := NewLevel3Book(10)
		rows := kraken.NewLevel3DataSlice([]byte(btcUSDLevel3ChecksumSnapshot))
		So(level3Book.Apply(rows[0], 1, 8), ShouldBeTrue)

		Convey("When the order's quantity is reduced in place", func() {
			before := level3Book.book("BTC/USD").bids["OTCFZG-YOE2Q-LQKNM3"]

			update := kraken.Level3Data{
				Symbol:   "BTC/USD",
				Type:     "update",
				Checksum: 404903681,
				Bids: []kraken.Level3Order{
					{Event: "modify", OrderID: "OTCFZG-YOE2Q-LQKNM3", LimitPrice: 44939.4, OrderQty: 0.5},
				},
			}
			valid := level3Book.Apply(update, 1, 8)

			Convey("Then the checksum validates and the order keeps its queue position", func() {
				So(valid, ShouldBeTrue)
				after := level3Book.book("BTC/USD").bids["OTCFZG-YOE2Q-LQKNM3"]
				So(after.sequence, ShouldEqual, before.sequence)
				So(after.orderQty, ShouldAlmostEqual, 0.5)
			})
		})

		Convey("When a modify targets an order the local book never saw", func() {
			update := kraken.Level3Data{
				Symbol: "BTC/USD",
				Type:   "update",
				Bids: []kraken.Level3Order{
					{Event: "modify", OrderID: "UNKNOWN-ORDER", LimitPrice: 44939.4, OrderQty: 0.5},
				},
			}
			valid := level3Book.Apply(update, 1, 8)

			Convey("Then the book is invalidated rather than silently accepting it", func() {
				So(valid, ShouldBeFalse)
				So(level3Book.Invalid("BTC/USD"), ShouldBeTrue)
			})
		})

		Convey("When an order carries an unrecognized event", func() {
			update := kraken.Level3Data{
				Symbol: "BTC/USD",
				Type:   "update",
				Bids: []kraken.Level3Order{
					{Event: "replace", OrderID: "OTCFZG-YOE2Q-LQKNM3", LimitPrice: 44939.4, OrderQty: 0.5},
				},
			}
			valid := level3Book.Apply(update, 1, 8)

			Convey("Then the book is invalidated", func() {
				So(valid, ShouldBeFalse)
			})
		})
	})

	Convey("Given a snapshot deeper than the retained depth", t, func() {
		level3Book := NewLevel3Book(3)
		snapshot := kraken.Level3Data{
			Symbol:   "ETH/USD",
			Type:     "snapshot",
			Checksum: 2862565709,
			Bids: []kraken.Level3Order{
				{OrderID: "b1", LimitPrice: 100, OrderQty: 1},
				{OrderID: "b2", LimitPrice: 99, OrderQty: 1},
				{OrderID: "b3", LimitPrice: 98, OrderQty: 1},
				{OrderID: "b4", LimitPrice: 97, OrderQty: 1},
			},
			Asks: []kraken.Level3Order{
				{OrderID: "a1", LimitPrice: 101, OrderQty: 1},
				{OrderID: "a2", LimitPrice: 102, OrderQty: 1},
				{OrderID: "a3", LimitPrice: 103, OrderQty: 1},
				{OrderID: "a4", LimitPrice: 104, OrderQty: 1},
			},
		}

		Convey("When it is applied", func() {
			valid := level3Book.Apply(snapshot, 0, 0)
			book := level3Book.book("ETH/USD")

			Convey("Then the checksum over the truncated top-3 levels validates and only that depth is retained", func() {
				So(valid, ShouldBeTrue)
				So(len(book.bids), ShouldEqual, 3)
				So(len(book.asks), ShouldEqual, 3)
				So(book.bids, ShouldNotContainKey, "b4")
				So(book.asks, ShouldNotContainKey, "a4")
			})
		})
	})

	Convey("Given a symbol demoted out of the trading tier", t, func() {
		level3Book := NewLevel3Book(10)
		rows := kraken.NewLevel3DataSlice([]byte(btcUSDLevel3ChecksumSnapshot))
		So(level3Book.Apply(rows[0], 1, 8), ShouldBeTrue)

		Convey("When Reset is called", func() {
			level3Book.Reset("BTC/USD")

			Convey("Then the symbol's local state is gone and it is no longer marked invalid", func() {
				So(level3Book.Invalid("BTC/USD"), ShouldBeFalse)
				_, _, ok := level3Book.TopOfBook("BTC/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkLevel3BookApply(b *testing.B) {
	rows := kraken.NewLevel3DataSlice([]byte(btcUSDLevel3ChecksumSnapshot))
	level3Book := NewLevel3Book(10)

	b.ReportAllocs()
	for b.Loop() {
		level3Book.Apply(rows[0], 1, 8)
	}
}
