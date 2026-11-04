package broker

import (
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/venue/kraken"
)

func TestCheckEntryPhysicalRejectsNilPrice(t *testing.T) {
	Convey("entry is blocked when the book cannot be priced", t, func() {
		decision := CheckEntryPhysical(nil, "Q/USD")
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "no price book")
	})
}

func TestCheckEntryPhysicalRejectsThinBids(t *testing.T) {
	Convey("entry is blocked when top bids have no cash depth", t, func() {
		const symbol = "THIN/USD"

		instrument := &Instrument{cache: &sync.Map{}}
		instrument.cache.Store(symbol, kraken.InstrumentPair{
			Symbol:         symbol,
			Quote:          "USD",
			QtyPrecision:   8,
			PricePrecision: 2,
			CostPrecision:  2,
		})

		price := &Price{
			Instrument: instrument,
			Books:      NewBookManager(),
		}
		decision := CheckEntryPhysical(price, symbol)
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "book too thin")
	})
}

func firmBook(symbol string) *BookManager {
	books := NewBookManager()
	bid := decimal.RequireFromString("100")
	ask := decimal.RequireFromString("100.20")
	qty := decimal.RequireFromString("2")

	books.Update(&market.Level3{
		Type: "snapshot",
		Data: []*market.Level3Data{{
			Symbol: symbol,
			Bids: []*market.Level3Order{{
				OrderID:    "b1",
				LimitPrice: &bid,
				OrderQty:   &qty,
				Timestamp:  time.Unix(1, 0),
			}},
			Asks: []*market.Level3Order{{
				OrderID:    "a1",
				LimitPrice: &ask,
				OrderQty:   &qty,
				Timestamp:  time.Unix(1, 0),
			}},
		}},
	})

	return books
}

func TestCheckEntryPhysicalTapeUptake(t *testing.T) {
	Convey("a firm book with a quiet tape is blocked", t, func() {
		const symbol = "TAPE/USD"

		price := &Price{Books: firmBook(symbol), tape: NewTape()}
		decision := CheckEntryPhysical(price, symbol)

		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "tape not picking up")
		So(decision.Reason, ShouldContainSubstring, "last window $0")
	})

	Convey("two dollars beating one dollar still misses the floor", t, func() {
		const symbol = "DUST/USD"
		now := time.Now()

		price := &Price{Books: firmBook(symbol), tape: NewTape()}
		price.tape.Note(symbol, 1, now.Add(-15*time.Minute))
		price.tape.Note(symbol, 2, now)

		decision := CheckEntryPhysical(price, symbol)
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "tape not picking up")
		So(decision.Reason, ShouldContainSubstring, "last window $2")
	})

	Convey("a fading tape is blocked even when the last window clears the floor", t, func() {
		const symbol = "FADE/USD"
		now := time.Now()

		price := &Price{Books: firmBook(symbol), tape: NewTape()}
		price.tape.Note(symbol, 300, now.Add(-15*time.Minute))
		price.tape.Note(symbol, 150, now)

		decision := CheckEntryPhysical(price, symbol)
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "last window $150 vs prior $300")
	})

	Convey("waking up above the floor passes the tape check", t, func() {
		const symbol = "WAKE/USD"

		price := &Price{Books: firmBook(symbol), tape: NewTape()}
		price.tape.Note(symbol, 200, time.Now())

		decision := CheckEntryPhysical(price, symbol)
		So(decision.Reason, ShouldNotContainSubstring, "tape not picking up")
	})
}
