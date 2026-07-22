package tests

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	marketsignal "github.com/theapemachine/symm/tests/fixtures/signal"
)

/*
TestValidator_Validate proves a rejected frame cannot mutate the reconstructed
book, checksum, or ticker state used by the next coherent frame.
*/
func TestValidator_Validate(t *testing.T) {
	Convey("Given a valid snapshot followed by one market update", t, func() {
		market := NewMarket(t.Context(), 1)
		market.signal.Bootstrap()
		snapshot, err := market.read(
			market.ticker,
			market.trade,
			market.book,
			market.level3,
		)
		So(err, ShouldBeNil)
		validator := NewValidator()
		So(validator.Validate(snapshot), ShouldBeNil)
		So(market.signal.Apply(MarketStep{
			Advance: time.Second,
			Actions: []MarketAction{{
				Kind: MarketTrade, Symbol: "SIM1/USD", Side: "buy", Qty: 1,
			}},
		}), ShouldBeNil)
		update, err := market.read(
			market.ticker,
			market.trade,
			market.book,
			market.level3,
		)
		So(err, ShouldBeNil)
		validLevel3 := append([]byte(nil), update.level3...)
		var corrupted map[string]any
		So(json.Unmarshal(update.level3, &corrupted), ShouldBeNil)
		corrupted["data"].([]any)[0].(map[string]any)["checksum"] = 1
		update.level3, err = json.Marshal(corrupted)
		So(err, ShouldBeNil)

		Convey("A bad checksum should not poison the valid retry", func() {
			So(validator.Validate(update), ShouldNotBeNil)
			update.level3 = validLevel3
			So(validator.Validate(update), ShouldBeNil)
		})
	})

	Convey("Given a valid snapshot and a book-only touch update", t, func() {
		market := NewMarket(t.Context(), 1)
		market.signal.Bootstrap()
		snapshot, err := market.read(
			market.ticker,
			market.trade,
			market.book,
			market.level3,
		)
		So(err, ShouldBeNil)
		validator := NewValidator()
		So(validator.Validate(snapshot), ShouldBeNil)
		So(market.signal.Apply(MarketStep{
			Advance: time.Second,
			Actions: []MarketAction{{
				Kind: MarketRefill, Symbol: "SIM1/USD", Side: "buy", Qty: 1,
			}},
		}), ShouldBeNil)
		update, err := market.read(
			market.ticker,
			market.trade,
			market.book,
			market.level3,
		)
		So(err, ShouldBeNil)

		for _, corruption := range []struct {
			name   string
			mutate func(map[string]any)
		}{
			{
				"a wrong channel",
				func(frame map[string]any) { frame["channel"] = "trade" },
			},
			{
				"a last price without a trade",
				func(frame map[string]any) {
					row := frame["data"].([]any)[0].(map[string]any)
					row["last"] = row["last"].(float64) + marketsignal.PriceIncrement
				},
			},
			{
				"a change without a trade",
				func(frame map[string]any) {
					row := frame["data"].([]any)[0].(map[string]any)
					row["change"] = row["change"].(float64) + marketsignal.PriceIncrement
				},
			},
			{
				"a timestamp that disagrees with the book",
				func(frame map[string]any) {
					row := frame["data"].([]any)[0].(map[string]any)
					row["timestamp"] = market.Now().Add(time.Second)
				},
			},
		} {
			corruption := corruption

			Convey("It should reject "+corruption.name+" without poisoning retry", func() {
				var frame map[string]any
				So(json.Unmarshal(update.ticker, &frame), ShouldBeNil)
				corruption.mutate(frame)
				corrupted, err := json.Marshal(frame)
				So(err, ShouldBeNil)
				rejected := update
				rejected.ticker = corrupted
				So(validator.Validate(rejected), ShouldNotBeNil)
				So(validator.Validate(update), ShouldBeNil)
			})
		}
	})

	Convey("Given a coherent snapshot containing an overflowing wire number", t, func() {
		market := NewMarket(t.Context(), 1)
		market.signal.Bootstrap()
		snapshot, err := market.read(
			market.ticker,
			market.trade,
			market.book,
			market.level3,
		)
		So(err, ShouldBeNil)
		corruptions := []struct {
			name       string
			channel    string
			collection string
			field      string
		}{
			{"ticker bid", "ticker", "", "bid"},
			{"ticker ask", "ticker", "", "ask"},
			{"ticker last", "ticker", "", "last"},
			{"trade price", "trade", "", "price"},
			{"L2 price", "book", "bids", "price"},
			{"L2 quantity", "book", "bids", "qty"},
			{"L3 price", "level3", "bids", "limit_price"},
			{"L3 quantity", "level3", "bids", "order_qty"},
		}

		for _, corruption := range corruptions {
			Convey("It should reject an invalid "+corruption.name+" without poisoning retry", func() {
				payload := snapshot.ticker

				switch corruption.channel {
				case "trade":
					payload = snapshot.trade
				case "book":
					payload = snapshot.book
				case "level3":
					payload = snapshot.level3
				}

				decoder := json.NewDecoder(bytes.NewReader(payload))
				decoder.UseNumber()
				var frame map[string]any
				So(decoder.Decode(&frame), ShouldBeNil)
				row := frame["data"].([]any)[0].(map[string]any)

				switch corruption.collection {
				case "":
					row[corruption.field] = json.Number("1e999")
				default:
					levels := row[corruption.collection].([]any)
					levels[0].(map[string]any)[corruption.field] = json.Number("1e999")
				}

				encoded, err := json.Marshal(frame)
				So(err, ShouldBeNil)
				rejected := snapshot

				switch corruption.channel {
				case "ticker":
					rejected.ticker = encoded
				case "trade":
					rejected.trade = encoded
				case "book":
					rejected.book = encoded
				case "level3":
					rejected.level3 = encoded
				}

				validator := NewValidator()
				So(validator.Validate(rejected), ShouldNotBeNil)
				So(validator.Validate(snapshot), ShouldBeNil)
			})
		}
	})
}

/*
TestValidator_bookChecksum proves the invariant gate applies Kraken's CRC to
the ten executable levels rather than every retained depth level.
*/
func TestValidator_bookChecksum(t *testing.T) {
	Convey("Given a reconstructed book deeper than Kraken's checksum window", t, func() {
		validator := NewValidator()
		validator.books["SIM1/USD"] = map[string]map[float64]float64{
			"asks": {},
			"bids": {},
		}

		for index := range bookChecksumDepth + 1 {
			validator.books["SIM1/USD"]["asks"][101+float64(index)] = 10
			validator.books["SIM1/USD"]["bids"][99-float64(index)] = 10
		}

		checksum := validator.bookChecksum("SIM1/USD")
		validator.books["SIM1/USD"]["asks"][101+bookChecksumDepth] = 20
		validator.books["SIM1/USD"]["bids"][99-bookChecksumDepth] = 20

		Convey("Depth beyond ten levels should not alter the CRC", func() {
			So(validator.bookChecksum("SIM1/USD"), ShouldEqual, checksum)
		})

		Convey("Executable depth inside ten levels should alter the CRC", func() {
			validator.books["SIM1/USD"]["asks"][101] = 20
			So(validator.bookChecksum("SIM1/USD"), ShouldNotEqual, checksum)
		})
	})
}
