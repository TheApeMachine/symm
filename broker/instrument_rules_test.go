package broker

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

func altPair(symbol string) market.InstrumentPair {
	return market.InstrumentPair{
		Symbol:       symbol,
		QtyIncrement: 0.00000001,
		QtyMin:       0.00000001,
		CostMin:      0.01,
	}
}

func TestInstrumentRulesCacheValidateOrder(t *testing.T) {
	Convey("Given instrument rules for BTC/EUR", t, func() {
		cache := NewInstrumentRulesCache(t.Context())
		cache.InstallPairForTest(market.InstrumentPair{
			Symbol:         "BTC/EUR",
			QtyIncrement:   0.0001,
			PriceIncrement: 0.01,
			QtyMin:         0.0001,
			CostMin:        10,
		})

		Convey("It accepts aligned market orders above minimums", func() {
			err := cache.ValidateOrder("BTC/EUR", 0.2, 100, trading.Market)

			So(err, ShouldBeNil)
		})

		Convey("It rejects sub-minimum quantity", func() {
			err := cache.ValidateOrder("BTC/EUR", 0.00001, 100, trading.Market)

			So(err, ShouldNotBeNil)
		})

		Convey("It rejects misaligned limit prices", func() {
			err := cache.ValidateOrder("BTC/EUR", 0.001, 100.001, trading.Limit)

			So(err, ShouldNotBeNil)
		})

		Convey("It rejects orders below cost minimum", func() {
			err := cache.ValidateOrder("BTC/EUR", 0.0001, 100, trading.Limit)

			So(err, ShouldNotBeNil)
		})
	})
}

func TestInstrumentRulesCacheAlignOrder(t *testing.T) {
	Convey("Given instrument increments", t, func() {
		cache := NewInstrumentRulesCache(t.Context())
		cache.InstallPairForTest(market.InstrumentPair{
			Symbol:         "BTC/EUR",
			QtyIncrement:   0.0001,
			PriceIncrement: 0.01,
		})

		qty, price := cache.AlignOrder("BTC/EUR", 0.00123, 100.019, trading.Limit)

		So(qty, ShouldAlmostEqual, 0.0012, 1e-9)
		So(price, ShouldAlmostEqual, 100.01, 1e-9)
	})
}

func TestInstrumentRulesCachePrepareOrder(t *testing.T) {
	Convey("Given altcoin instrument increments", t, func() {
		cache := NewInstrumentRulesCache(t.Context())
		cache.InstallPairForTest(altPair("LTC/EUR"))
		cache.InstallPairForTest(altPair("XRP/EUR"))

		sizedQuantity := func(notionalEUR, price float64) float64 {
			return notionalEUR / price
		}

		Convey("It rejects misaligned quantity before rounding", func() {
			quantity := sizedQuantity(50, 94.523) * 0.99

			err := cache.ValidateOrder("LTC/EUR", quantity, 0, trading.Limit)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not aligned to increment")
		})

		Convey("It accepts the same quantity after rounding down to increment", func() {
			quantity := sizedQuantity(50, 94.523) * 0.99

			alignedQty, _, err := cache.PrepareOrder("LTC/EUR", quantity, 0, trading.Limit)

			So(err, ShouldBeNil)
			So(alignedQty, ShouldBeLessThan, quantity)
			So(isAligned(alignedQty, 0.00000001), ShouldBeTrue)
		})

		Convey("It preserves quantities already on the increment lattice", func() {
			for symbol, quantity := range map[string]float64{
				"LTC/EUR": 0.52868094,
				"XRP/EUR": 20.90759887,
			} {
				alignedQty, _, err := cache.PrepareOrder(symbol, quantity, 0, trading.Limit)

				So(err, ShouldBeNil)
				So(alignedQty, ShouldAlmostEqual, quantity, 1e-12)
				So(isAligned(alignedQty, 0.00000001), ShouldBeTrue)
			}
		})

		Convey("It rejects quantity that rounds to zero", func() {
			_, _, err := cache.PrepareOrder("LTC/EUR", 0.000000001, 0, trading.Limit)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "rounds to zero below increment")
		})

		Convey("It aligns limit prices before validating them", func() {
			cache.InstallPairForTest(market.InstrumentPair{
				Symbol:         "BTC/EUR",
				QtyIncrement:   0.0001,
				PriceIncrement: 0.01,
				QtyMin:         0.0001,
				CostMin:        10,
			})

			err := cache.ValidateOrder("BTC/EUR", 0.0012, 100.019, trading.Limit)

			So(err, ShouldNotBeNil)

			alignedQty, alignedPrice, err := cache.PrepareOrder(
				"BTC/EUR", 0.2, 100.019, trading.Limit,
			)

			So(err, ShouldBeNil)
			So(alignedQty, ShouldAlmostEqual, 0.2, 1e-9)
			So(alignedPrice, ShouldAlmostEqual, 100.01, 1e-9)
			So(isAligned(alignedPrice, 0.01), ShouldBeTrue)
		})
	})
}

func TestInstrumentRulesCacheIsAligned(t *testing.T) {
	Convey("Given increment lattice values", t, func() {
		increment := 0.00000001

		Convey("It treats float-stored lattice values as aligned", func() {
			So(isAligned(0.52868094, increment), ShouldBeTrue)
			So(isAligned(20.90759887, increment), ShouldBeTrue)
		})

		Convey("It rejects off-lattice sizing products", func() {
			quantity := (50.0 / 94.523) * 0.99

			So(isAligned(quantity, increment), ShouldBeFalse)
			So(
				isAligned(roundDownToIncrement(quantity, increment), increment),
				ShouldBeTrue,
			)
		})
	})
}

func BenchmarkInstrumentRulesCachePrepareOrder(b *testing.B) {
	cache := NewInstrumentRulesCache(b.Context())
	cache.InstallPairForTest(altPair("LTC/EUR"))
	quantity := (50.0 / 94.523) * 0.99

	for b.Loop() {
		_, _, _ = cache.PrepareOrder("LTC/EUR", quantity, 0, trading.Limit)
	}
}

func BenchmarkInstrumentRulesCacheIsAligned(b *testing.B) {
	increment := 0.00000001
	quantity := (50.0 / 94.523) * 0.99

	for b.Loop() {
		_ = isAligned(quantity, increment)
	}
}

func TestInstrumentRulesCachePrepareOrderStressSized(t *testing.T) {
	Convey("Given stress-scaled sizing output", t, func() {
		cache := NewInstrumentRulesCache(t.Context())
		cache.InstallPairForTest(altPair("LTC/EUR"))

		base := 50.0 / 94.523
		stress := SymbolStress{
			HawkesCategory: perspectives.CategorySaturation,
			HawkesSNR:      1,
		}
		quantity := stress.EntryQuantity(base)

		Convey("ValidateOrder alone rejects the stressed quantity", func() {
			err := cache.ValidateOrder("LTC/EUR", quantity, 0, trading.Limit)

			So(err, ShouldNotBeNil)
		})

		Convey("PrepareOrder accepts after rounding", func() {
			alignedQty, _, err := cache.PrepareOrder("LTC/EUR", quantity, 0, trading.Limit)

			So(err, ShouldBeNil)
			So(alignedQty, ShouldBeGreaterThan, 0)
			So(math.Abs(quantity-alignedQty), ShouldBeLessThan, 0.00000001)
		})
	})
}
