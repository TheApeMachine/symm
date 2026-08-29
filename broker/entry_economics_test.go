package broker

import (
	"math"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestPriceEntryCost(t *testing.T) {
	Convey("Given current quotes and a valid taker fee", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 5)
		cost, err := price.EntryCost("EDGE/USD", decimal.NewFromFloat64(1))

		Convey("It should report only present execution and fee facts", func() {
			So(err, ShouldBeNil)
			So(cost.EntryPrice.Float64(), ShouldAlmostEqual, 101, 1e-12)
			So(cost.Midpoint.Float64(), ShouldAlmostEqual, 100.5, 1e-12)
			So(cost.Spread.Float64(), ShouldAlmostEqual, 0.5, 1e-12)
			So(cost.Impact.Sign(), ShouldEqual, 0)
			So(cost.EntryFee.Float64(), ShouldAlmostEqual, 101*0.0025, 1e-12)
			So(cost.BreakEven.Cmp(cost.EntryPrice), ShouldBeGreaterThan, 0)
			So(cost.RoundTripFees.Sign(), ShouldEqual, 1)
		})
	})

	Convey("Given a request exactly at the visible ask quantity", t, func() {
		// entryDepthVWAP always returns unavailable now (no full-depth book to
		// walk), so EntryCost always takes its ticker-level fallback path,
		// pricing entirely off tick.Ask/tick.AskQty rather than a walked depth
		// chain.
		price := entryEconomicsFixture(t, 101, 100, 5)
		cost, err := price.EntryCost("EDGE/USD", decimal.NewFromFloat64(5))

		Convey("It should price entirely off the best ask, with zero impact", func() {
			So(err, ShouldBeNil)
			So(cost.EntryPrice.Float64(), ShouldAlmostEqual, 101, 1e-12)
			So(cost.BestAsk.Float64(), ShouldAlmostEqual, 101, 1e-12)
			So(cost.Impact.Sign(), ShouldEqual, 0)
		})

		Convey("It should reject quantity beyond the visible ask quantity", func() {
			_, err := price.EntryCost("EDGE/USD", decimal.NewFromFloat64(6))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "visible ask quantity")
		})
	})

	Convey("Given crossed current quotes", t, func() {
		price := entryEconomicsFixture(t, 99, 100, 5)
		_, err := price.EntryCost("EDGE/USD", decimal.NewFromFloat64(1))
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "crossed")
	})

	Convey("Given a high-priced asset with a sub-tick-sized quantity", t, func() {
		quantity := 0.00051057
		price := entryEconomicsFixture(t, 64951.1, 64951.0, quantity)
		cost, err := price.EntryCost("EDGE/USD", decimal.NewFromFloat64(quantity))
		So(err, ShouldBeNil)
		So(cost.GrossNotional.Sign(), ShouldEqual, 1)
	})
}

func TestPriceExecutableQuantity(t *testing.T) {
	// ExecutableQuantity has no full-depth book to walk, so it always prices
	// off tick.AskQty alone rather than a walked ask-side depth chain.
	Convey("Given a request the visible ask quantity can fully cover", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 5)

		Convey("It should preserve the request unchanged", func() {
			quantity, err := price.ExecutableQuantity(
				"EDGE/USD", decimal.NewFromFloat64(4),
			)
			So(err, ShouldBeNil)
			So(quantity.Float64(), ShouldAlmostEqual, 4, 1e-12)
		})

		Convey("It should cap a request beyond the visible ask quantity at that quantity", func() {
			quantity, err := price.ExecutableQuantity(
				"EDGE/USD", decimal.NewFromFloat64(9),
			)
			So(err, ShouldBeNil)
			So(quantity.Float64(), ShouldAlmostEqual, 5, 1e-12)
		})
	})

	Convey("Given ticker quotes with a smaller visible ask quantity", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 3)
		quantity, err := price.ExecutableQuantity(
			"EDGE/USD", decimal.NewFromFloat64(5),
		)
		So(err, ShouldBeNil)
		So(quantity.Float64(), ShouldAlmostEqual, 3, 1e-12)
	})

	Convey("Given crossed best quotes", t, func() {
		price := entryEconomicsFixture(t, 99, 100, 5)
		_, err := price.ExecutableQuantity("EDGE/USD", decimal.NewFromFloat64(1))
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "crossed best quotes")
	})

	Convey("Given a non-finite or non-positive request", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 3)
		_, err := price.ExecutableQuantity("EDGE/USD", decimal.NewFromFloat64(0))
		So(err, ShouldNotBeNil)
		So(math.IsNaN(0), ShouldBeFalse)
	})
}

func entryEconomicsFixture(
	testingTB testing.TB,
	ask float64,
	bid float64,
	quoteQuantity float64,
) *Price {
	testingTB.Helper()
	price, _ := newPriceSurface(testingTB, "EDGE/USD")
	price.Update(&kraken.TickerData{
		Symbol: "EDGE/USD",
		Ask:    decimal.NewFromFloat64(ask),
		AskQty: quoteQuantity,
		Bid:    decimal.NewFromFloat64(bid),
		BidQty: quoteQuantity,
	})

	return price
}

func BenchmarkPriceEntryCost(b *testing.B) {
	price := entryEconomicsFixture(b, 64951.1, 64951.0, 0.00051057)
	quantity := decimal.NewFromFloat64(0.00051057)

	for b.Loop() {
		if _, err := price.EntryCost("EDGE/USD", quantity); err != nil {
			b.Fatal(err)
		}
	}
}
