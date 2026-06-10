package market

import (
	"encoding/json"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestKrakenBookChecksumOfficialExample(t *testing.T) {
	Convey("Given Kraken book checksum documentation snapshot", t, func() {
		raw := json.RawMessage(`[{
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
		}]`)

		updates := BookUpdates{}

		So(sonic.Unmarshal(raw, &updates), ShouldBeNil)
		So(len(updates), ShouldEqual, 1)

		update := updates[0]
		computed := checksumBook(update, 10)

		Convey("It should match the published CRC32 checksum", func() {
			So(uint32(update.Checksum), ShouldEqual, computed)
			So(computed, ShouldEqual, uint32(3310070434))
		})

		Convey("It should apply through BookStore without mismatch", func() {
			store := NewBookStore(10)
			update.Type = "snapshot"

			So(store.Apply(update), ShouldBeNil)
		})
	})
}

func TestBookStoreUpdateBeforeSnapshot(t *testing.T) {
	Convey("Given a delta before any snapshot", t, func() {
		store := NewBookStore(10)

		update := &BookUpdate{
			Symbol: "ETH/USD",
			Type:   "update",
			Bids:   []BookLevel{{Price: 3000, Qty: 1}},
		}

		Convey("It should reject without poisoning state", func() {
			So(store.Apply(update), ShouldNotBeNil)

			_, ok := store.Snapshot("ETH/USD")
			So(ok, ShouldBeFalse)
		})
	})
}

func TestMergeLevelsCollapsesEqualPrices(t *testing.T) {
	Convey("Given two wire forms for the same price", t, func() {
		current := []BookLevel{{
			Price:     45285.2,
			Qty:       1,
			priceWire: "45285.20",
			qtyWire:   "1.00000000",
		}}

		delta := []BookLevel{{
			Price:     45285.2,
			Qty:       2,
			priceWire: "45285.2",
			qtyWire:   "2",
		}}

		merged := mergeLevels(current, delta, true)

		Convey("It should keep one level with the delta quantity and wire", func() {
			So(len(merged), ShouldEqual, 1)
			So(merged[0].Qty, ShouldEqual, 2)
			So(merged[0].qtyWire, ShouldEqual, "2")
		})
	})
}

func TestKrakenBookChecksumNumericJSON(t *testing.T) {
	Convey("Given Kraken doc snapshot with full-precision numeric fields", t, func() {
		raw := []byte(`[{
			"symbol":"BTC/USD",
			"bids":[
				{"price":45283.5,"qty":0.10000000},
				{"price":45283.4,"qty":1.54582015},
				{"price":45282.1,"qty":0.10000000},
				{"price":45281.0,"qty":0.10000000},
				{"price":45280.3,"qty":1.54592586},
				{"price":45279.0,"qty":0.07990000},
				{"price":45277.6,"qty":0.03310103},
				{"price":45277.5,"qty":0.30000000},
				{"price":45277.3,"qty":1.54602737},
				{"price":45276.6,"qty":0.15445238}
			],
			"asks":[
				{"price":45285.2,"qty":0.00100000},
				{"price":45286.4,"qty":1.54571953},
				{"price":45286.6,"qty":1.54571109},
				{"price":45289.6,"qty":1.54560911},
				{"price":45290.2,"qty":0.15890660},
				{"price":45291.8,"qty":1.54553491},
				{"price":45294.7,"qty":0.04454749},
				{"price":45296.1,"qty":0.35380000},
				{"price":45297.5,"qty":0.09945542},
				{"price":45299.5,"qty":0.18772827}
			],
			"checksum":3310070434
		}]`)

		updates := BookUpdates{}

		So(sonic.Unmarshal(raw, &updates), ShouldBeNil)

		computed := checksumBook(updates[0], 10)

		Convey("It should match when JSON numbers preserve trailing zeros on the wire", func() {
			So(computed, ShouldEqual, uint32(3310070434))
		})
	})
}

func TestWireChecksumToken(t *testing.T) {
	Convey("Given Kraken wire formatting rules", t, func() {
		Convey("It should strip decimals and leading zeros from qty", func() {
			So(wireChecksumToken("0.00100000"), ShouldEqual, "100000")
			So(wireChecksumToken("45285.2"), ShouldEqual, "452852")
		})
	})
}

func TestBookLevelPreservesWirePrecision(t *testing.T) {
	Convey("Given string-encoded book levels from Kraken", t, func() {
		level := BookLevel{}

		So(sonic.Unmarshal([]byte(`{"price":"45290.2","qty":"0.15890660"}`), &level), ShouldBeNil)

		Convey("It should keep wire tokens exact for checksum", func() {
			So(level.priceChecksumToken(), ShouldEqual, "452902")
			So(level.qtyChecksumToken(), ShouldEqual, "15890660")
		})
	})
}
