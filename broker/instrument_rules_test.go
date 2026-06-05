package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

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
