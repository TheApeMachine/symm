package broker

import (
	"strings"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/venue/kraken"
)

func testInstrument(tb testing.TB, extra ...kraken.InstrumentPair) *Instrument {
	tb.Helper()

	normalizer := spot.NewNormalizer()
	seedPair(normalizer, testSymbol)

	base, quote, _ := strings.Cut(testSymbol, "/")
	qtyMin := decimal.RequireFromString("0.01")
	costMin := decimal.RequireFromString("1")
	step := decimal.RequireFromString("0.01")
	fee := decimal.RequireFromString("0.40")

	instrument := &Instrument{
		cache: &sync.Map{}, takerFees: &sync.Map{},
		quote: quote, symbols: []string{testSymbol}, normalizer: normalizer,
	}
	instrument.cache.Store(testSymbol, kraken.InstrumentPair{
		Symbol: testSymbol, Base: base, Quote: quote, Status: "online",
		QtyPrecision: 8, PricePrecision: 2, CostPrecision: 2,
		QtyMin: &qtyMin, CostMin: &costMin, QtyIncrement: &step, TickSize: &step,
	})
	instrument.takerFees.Store(testSymbol, &fee)

	for _, pair := range extra {
		symbol := normalizer.Name(pair.Symbol)
		if pair.Quote == quote && pair.Status == "online" {
			instrument.symbols = append(instrument.symbols, symbol)
			instrument.cache.Store(symbol, pair)
		}
	}

	return instrument
}

func TestInstrumentCache(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)

		Convey("When Cache injects online quote pairs", func() {
			ethMin := decimal.RequireFromString("0.01")
			instrument.Cache([]kraken.InstrumentPair{
				{Symbol: "ETH/USD", Base: "ETH", Quote: "USD", Status: "online", QtyMin: &ethMin},
				{Symbol: "ETH/USD", Base: "ETH", Quote: "EUR", Status: "online", QtyMin: &ethMin},
				{Symbol: "OFF/USD", Base: "OFF", Quote: "USD", Status: "cancel_only", QtyMin: &ethMin},
			})

			Convey("Only online USD pairs land in the map", func() {
				So(instrument.Has("ETH/USD"), ShouldBeTrue)
				So(instrument.Has("OFF/USD"), ShouldBeFalse)
				So(len(instrument.Pairs()), ShouldEqual, 2)
			})
		})
	})
}

func TestInstrumentHas(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)

		Convey("When the symbol is in the cache", func() {
			So(instrument.Has(testSymbol), ShouldBeTrue)
		})

		Convey("When the symbol is missing", func() {
			So(instrument.Has("MISSING/USD"), ShouldBeFalse)
		})
	})
}

func TestInstrumentSpotOpen(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)

		Convey("When the pair is online", func() {
			So(instrument.SpotOpen(testSymbol), ShouldBeTrue)
		})

		Convey("When the symbol is empty", func() {
			So(instrument.SpotOpen(""), ShouldBeFalse)
		})

		Convey("When the pair is not cached", func() {
			So(instrument.SpotOpen("MISSING/USD"), ShouldBeFalse)
		})
	})
}

func TestInstrumentPair(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)

		Convey("When the symbol exists", func() {
			pair := instrument.Pair(testSymbol)

			Convey("QtyMin and status match the snapshot", func() {
				So(pair.Symbol, ShouldEqual, testSymbol)
				So(pair.Status, ShouldEqual, "online")
				So(pair.QtyMin.String(), ShouldEqual, "0.01")
			})
		})

		Convey("When the symbol is missing", func() {
			pair := instrument.Pair("MISSING/USD")

			Convey("Pair is empty", func() {
				So(pair.Symbol, ShouldEqual, "")
			})
		})
	})
}

func TestInstrumentSymbols(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)

		Convey("When Symbols is called", func() {
			syms := instrument.Symbols()
			syms[0] = "mutated"

			Convey("A clone is returned", func() {
				So(instrument.Symbols()[0], ShouldEqual, testSymbol)
			})
		})
	})
}

func TestInstrumentTakerFee(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)

		Convey("When the fee table has the symbol", func() {
			fee := instrument.TakerFee(testSymbol)

			Convey("Taker fee is 0.40%", func() {
				So(fee, ShouldNotBeNil)
				So(fee.String(), ShouldEqual, "0.4")
			})
		})

		Convey("When the symbol is unknown", func() {
			So(instrument.TakerFee("MISSING/USD"), ShouldBeNil)
		})
	})
}

func TestInstrumentPairs(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)
		ethMin := decimal.RequireFromString("0.01")
		instrument.Cache([]kraken.InstrumentPair{
			{Symbol: "ETH/USD", Base: "ETH", Quote: "USD", Status: "online", QtyMin: &ethMin},
		})

		Convey("When Pairs walks the cache", func() {
			pairs := instrument.Pairs()

			Convey("Both BTC and ETH are present", func() {
				So(len(pairs), ShouldEqual, 2)
			})
		})
	})
}

func BenchmarkInstrumentCache(b *testing.B) {
	instrument := testInstrument(b)
	ethMin := decimal.RequireFromString("0.01")
	pairs := []kraken.InstrumentPair{
		{Symbol: "ETH/USD", Base: "ETH", Quote: "USD", Status: "online", QtyMin: &ethMin},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		instrument.Cache(pairs)
	}
}

func BenchmarkInstrumentHas(b *testing.B) {
	instrument := testInstrument(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = instrument.Has(testSymbol)
	}
}

func BenchmarkInstrumentSpotOpen(b *testing.B) {
	instrument := testInstrument(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = instrument.SpotOpen(testSymbol)
	}
}

func BenchmarkInstrumentPair(b *testing.B) {
	instrument := testInstrument(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = instrument.Pair(testSymbol)
	}
}

func BenchmarkInstrumentSymbols(b *testing.B) {
	instrument := testInstrument(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = instrument.Symbols()
	}
}

func BenchmarkInstrumentTakerFee(b *testing.B) {
	instrument := testInstrument(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = instrument.TakerFee(testSymbol)
	}
}

func BenchmarkInstrumentPairs(b *testing.B) {
	instrument := testInstrument(b)
	ethMin := decimal.RequireFromString("0.01")
	instrument.Cache([]kraken.InstrumentPair{
		{Symbol: "ETH/USD", Base: "ETH", Quote: "USD", Status: "online", QtyMin: &ethMin},
	})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = instrument.Pairs()
	}
}
