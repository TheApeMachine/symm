package broker

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

func TestPriceEntryEconomics(t *testing.T) {
	Convey("Given a forecast larger than the measured round-trip cost", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 5)

		Convey("It should preserve every cost in compatible midpoint-return units", func() {
			economics, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(5),
				0.02,
			)

			So(err, ShouldBeNil)
			So(economics.ExpectedReturn.Float64(), ShouldAlmostEqual, 0.02, 1e-12)
			So(economics.ExpectedSpread.Float64(), ShouldAlmostEqual, 1.0/100.5, 1e-12)
			So(economics.ExpectedFees.Float64(), ShouldAlmostEqual,
				(101*0.0025+102.01*0.0025)/100.5, 1e-12)
			So(economics.ExpectedImpact.Sign(), ShouldEqual, 0)
			So(economics.NetReturn.Sign(), ShouldEqual, 1)
		})
	})

	Convey("Given a positive forecast that is smaller than spread and fees", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 5)

		Convey("It should expose a negative executable return", func() {
			economics, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				0.01,
			)

			So(err, ShouldBeNil)
			So(economics.ExpectedReturn.Sign(), ShouldEqual, 1)
			So(economics.NetReturn.Sign(), ShouldEqual, -1)
		})
	})

	Convey("Given the exact analytical break-even return", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 5)
		feeRate := 0.0025
		midpoint := (101.0 + 100.0) / 2.0
		breakEven := (101.0*(1.0+feeRate)/(1.0-feeRate) - 100.0) / midpoint
		returnQuantum := math.Pow10(-decimal.DefaultScale)

		Convey("The adjacent floating-point returns should fall on opposite sides", func() {
			below, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				breakEven-returnQuantum,
			)
			So(err, ShouldBeNil)

			above, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				breakEven+returnQuantum,
			)
			So(err, ShouldBeNil)
			So(below.NetReturn.Sign(), ShouldBeLessThanOrEqualTo, 0)
			So(above.NetReturn.Sign(), ShouldBeGreaterThanOrEqualTo, 0)
			So(below.NetReturn.Cmp(above.NetReturn), ShouldBeLessThan, 0)
		})
	})

	Convey("Given a quantity at the complete best-quote boundary", t, func() {
		price := entryEconomicsFixture(t, 100.02, 100, 5)

		Convey("It should accept the boundary and refuse the next representable quantity", func() {
			_, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(5),
				0.02,
			)
			So(err, ShouldBeNil)

			_, err = price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(math.Nextafter(5, math.Inf(1))),
				0.02,
			)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "depth impact required")
		})
	})

	Convey("Given a high-priced asset with a sub-tick-sized quantity", t, func() {
		quantity := 0.00051057
		price := entryEconomicsFixture(t, 64951.1, 64951.0, quantity)

		Convey("It should compare the quantity without rounding it to zero", func() {
			economics, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(quantity),
				0.01,
			)

			So(err, ShouldBeNil)
			So(economics.NetReturn.Sign(), ShouldEqual, 1)
		})
	})

	Convey("Given crossed quotes", t, func() {
		price := entryEconomicsFixture(t, 99, 100, 5)

		Convey("It should refuse to invent executable long economics", func() {
			_, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				0.02,
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "crossed")
		})
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

func BenchmarkPriceEntryEconomics(b *testing.B) {
	price := entryEconomicsFixture(b, 64951.1, 64951.0, 0.00051057)
	quantity := decimal.NewFromFloat64(0.00051057)

	for b.Loop() {
		if _, err := price.EntryEconomics("EDGE/USD", quantity, 0.01); err != nil {
			b.Fatal(err)
		}
	}
}
