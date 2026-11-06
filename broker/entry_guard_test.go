package broker

import (
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

func TestCheckEntryPhysicalRejectsNilPrice(t *testing.T) {
	Convey("entry is blocked when the book cannot be priced", t, func() {
		decision := CheckEntryPhysical(nil, "Q/USD")
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "no price book")
	})
}

func TestCheckEntryPeakBlocksLocalHigh(t *testing.T) {
	Convey("entry is blocked when last sits at the top of the 24h range", t, func() {
		prev := system.Cfg.Risk.MaxEntryPeakFrac
		system.Cfg.Risk.MaxEntryPeakFrac = 0.90
		Reset(func() { system.Cfg.Risk.MaxEntryPeakFrac = prev })

		price := &Price{}
		price.SetTicker("PEAK/USD", 1e6, 0, 100, 0, 95)

		decision := CheckEntryPeak(price, "PEAK/USD")
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "local high")
		So(decision.Reason, ShouldContainSubstring, "95%")

		physical := CheckEntryPhysical(price, "PEAK/USD")
		So(physical.Allowed, ShouldBeFalse)
		So(physical.Reason, ShouldContainSubstring, "local high")
	})

	Convey("entry clears the peak gate when last is under the cap", t, func() {
		prev := system.Cfg.Risk.MaxEntryPeakFrac
		system.Cfg.Risk.MaxEntryPeakFrac = 0.90
		Reset(func() { system.Cfg.Risk.MaxEntryPeakFrac = prev })

		price := &Price{}
		price.SetTicker("OK/USD", 1e6, 0, 100, 0, 50)

		So(CheckEntryPeak(price, "OK/USD").Allowed, ShouldBeTrue)
	})

	Convey("missing 24h range vetoes while the peak cap is enabled", t, func() {
		prev := system.Cfg.Risk.MaxEntryPeakFrac
		system.Cfg.Risk.MaxEntryPeakFrac = 0.90
		Reset(func() { system.Cfg.Risk.MaxEntryPeakFrac = prev })

		decision := CheckEntryPeak(&Price{}, "MISSING/USD")
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "24h range unavailable")
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
		price.SetTicker(symbol, 1e6, 1.0, 120, 80, 100)

		decision := CheckEntryPhysical(price, symbol)
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "book too thin")
	})
}

func TestCheckEntryMoverBlocksFlatDay(t *testing.T) {
	Convey("entry is blocked when the day move is flat", t, func() {
		prev := system.Cfg.Risk.MinMoverChangePct
		system.Cfg.Risk.MinMoverChangePct = 0.5
		Reset(func() { system.Cfg.Risk.MinMoverChangePct = prev })

		price := &Price{}
		price.SetTicker("FLAT/USD", 1e6, 0.1, 120, 80, 100)

		decision := CheckEntryMover(price, "FLAT/USD")
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "day move too flat")
	})

	Convey("entry is blocked when 24h volume is thin", t, func() {
		prev := system.Cfg.Risk.MinMoverVolumeUSD
		system.Cfg.Risk.MinMoverVolumeUSD = 50_000
		Reset(func() { system.Cfg.Risk.MinMoverVolumeUSD = prev })

		price := &Price{}
		price.SetTicker("DUST/USD", 1_000, 2.0, 120, 80, 100)

		decision := CheckEntryMover(price, "DUST/USD")
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "volume too thin")
	})

	Convey("a real mover clears the rail", t, func() {
		price := &Price{}
		price.SetTicker("MOVE/USD", 1e6, 2.5, 120, 80, 100)

		So(CheckEntryMover(price, "MOVE/USD").Allowed, ShouldBeTrue)
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

// midRangePrice seeds peak + mover so later rails (tape, book) can run.
func midRangePrice(symbol string) *Price {
	price := &Price{Books: firmBook(symbol), tape: NewTape()}
	price.SetTicker(symbol, 1e6, 1.0, 120, 80, 100)
	return price
}

func TestCheckEntryPhysicalTapeUptake(t *testing.T) {
	Convey("a firm book with a quiet tape is blocked", t, func() {
		const symbol = "TAPE/USD"

		price := midRangePrice(symbol)
		decision := CheckEntryPhysical(price, symbol)

		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "tape not picking up")
		So(decision.Reason, ShouldContainSubstring, "last window $0")
	})

	Convey("two dollars beating one dollar still misses the floor", t, func() {
		const symbol = "DUST/USD"
		now := time.Now()

		price := midRangePrice(symbol)
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

		price := midRangePrice(symbol)
		price.tape.Note(symbol, 300, now.Add(-15*time.Minute))
		price.tape.Note(symbol, 150, now)

		decision := CheckEntryPhysical(price, symbol)
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "last window $150 vs prior $300")
	})

	Convey("waking up above the floor passes the tape check", t, func() {
		const symbol = "WAKE/USD"

		price := midRangePrice(symbol)
		price.tape.Note(symbol, 200, time.Now())

		decision := CheckEntryPhysical(price, symbol)
		So(decision.Reason, ShouldNotContainSubstring, "tape not picking up")
	})
}
