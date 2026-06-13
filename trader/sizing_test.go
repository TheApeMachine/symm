package trader

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/logic"
)

func sampleMeasurements(confidence float64, surprise float64) []logic.Measurement {
	return []logic.Measurement{
		logic.NewMeasurement(
			logic.SourceHawkes,
			"BTC/USD",
			50_000,
			1,
			1,
			1,
			1,
			logic.CategoryFrenzy,
			logic.RegimeTypeNone,
			logic.PositionTypeNone,
			confidence,
			surprise,
		),
		logic.NewMeasurement(
			logic.SourceCVD,
			"BTC/USD",
			50_000,
			0.8,
			1,
			1,
			1,
			logic.CategoryAggressiveDrive,
			logic.RegimeTypeNone,
			logic.PositionTypeNone,
			confidence*0.9,
			surprise*0.8,
		),
	}
}

func TestEntrySlotFraction(test *testing.T) {
	tradingConfig := config.TradingConfig{
		MaxConcurrentPositions: 4,
		OpportunitySlotCount:   2,
	}
	thresholdConfig := config.ThresholdConfig{
		EntryConfidenceBaseline:   0.55,
		TurbulenceConfidenceScale: 0.30,
		EntrySurpriseBaseline:     1.0,
	}

	Convey("Given a six-slot envelope and a strong measurement spectrum", test, func() {
		holdings := logic.NewHoldings()
		measurements := sampleMeasurements(0.9, 1.2)

		fraction, err := EntrySlotFraction(
			holdings,
			measurements,
			thresholdConfig,
			tradingConfig,
			0,
			false,
		)

		Convey("It should derive a positive first-slot fraction", func() {
			So(err, ShouldBeNil)
			So(fraction, ShouldBeGreaterThan, 0)
			So(fraction, ShouldBeLessThanOrEqualTo, 1)
		})
	})

	Convey("Given a weaker confidence spectrum", test, func() {
		holdings := logic.NewHoldings()
		strongMeasurements := sampleMeasurements(0.9, 1.2)
		weakMeasurements := sampleMeasurements(0.6, 0.8)

		strongFraction, strongErr := EntrySlotFraction(
			holdings,
			strongMeasurements,
			thresholdConfig,
			tradingConfig,
			0,
			false,
		)
		weakFraction, weakErr := EntrySlotFraction(
			holdings,
			weakMeasurements,
			thresholdConfig,
			tradingConfig,
			0,
			false,
		)

		Convey("It should size stronger evidence larger than weaker evidence", func() {
			So(strongErr, ShouldBeNil)
			So(weakErr, ShouldBeNil)
			So(strongFraction, ShouldBeGreaterThan, weakFraction)
		})
	})

	Convey("Given a lower-tier slot against stronger open positions", test, func() {
		holdings := logic.NewHoldings()
		holdings.SetPosition("AAA/USD", 1, 0.95, false)
		holdings.SetPosition("BBB/USD", 1, 0.94, false)
		holdings.SetPosition("CCC/USD", 1, 0.93, false)
		holdings.SetPosition("DDD/USD", 1, 0.92, false)

		primaryFraction, primaryErr := EntrySlotFraction(
			logic.NewHoldings(),
			sampleMeasurements(0.85, 1.0),
			thresholdConfig,
			tradingConfig,
			0,
			false,
		)
		secondaryFraction, secondaryErr := EntrySlotFraction(
			holdings,
			sampleMeasurements(0.85, 1.0),
			thresholdConfig,
			tradingConfig,
			0,
			false,
		)

		Convey("It should shrink size relative to open-book rank", func() {
			So(primaryErr, ShouldBeNil)
			So(secondaryErr, ShouldBeNil)
			So(secondaryFraction, ShouldBeLessThan, primaryFraction)
		})
	})
}

func TestOrderQuantityFromFraction(t *testing.T) {
	Convey("Given a paper wallet and reference price", t, func() {
		quantity, err := OrderQuantityFromFraction(200, 0.2, 50_000)

		Convey("It should convert notional into base quantity", func() {
			So(err, ShouldBeNil)
			So(quantity, ShouldAlmostEqual, 0.0008, 1e-12)
		})
	})
}

func BenchmarkEntrySlotFraction(b *testing.B) {
	holdings := logic.NewHoldings()
	measurements := sampleMeasurements(0.9, 1.2)
	tradingConfig := config.TradingConfig{
		MaxConcurrentPositions: 4,
		OpportunitySlotCount:   2,
	}
	thresholdConfig := config.ThresholdConfig{
		EntryConfidenceBaseline:   0.55,
		TurbulenceConfidenceScale: 0.30,
		EntrySurpriseBaseline:     1.0,
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = EntrySlotFraction(
			holdings,
			measurements,
			thresholdConfig,
			tradingConfig,
			0,
			false,
		)
	}
}

func BenchmarkOrderQuantityFromFraction(b *testing.B) {
	for b.Loop() {
		_, _ = OrderQuantityFromFraction(200, 0.2, 50_000)
	}
}

func TestQuoteWalletBalance(t *testing.T) {
	Convey("Given paper wallet config", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		balance, err := QuoteWalletBalance("paper")

		Convey("It should return the configured quote balance", func() {
			So(err, ShouldBeNil)
			So(balance, ShouldEqual, 200)
		})
	})
}

func TestCapitalProvider(t *testing.T) {
	Convey("Given paper wallet config", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		provider, err := NewCapitalProvider(config.TradingConfig{Model: "paper"})

		Convey("It should provide available quote balance", func() {
			So(err, ShouldBeNil)
			So(provider, ShouldNotBeNil)

			available, balanceErr := provider.AvailableQuoteBalance(
				context.Background(),
				"USD",
			)

			So(balanceErr, ShouldBeNil)
			So(available, ShouldEqual, 200)
		})
	})

	Convey("Given live trading without an account provider", t, func() {
		provider, err := NewCapitalProvider(config.TradingConfig{Model: "live"})

		Convey("It should fail before entries can be sized", func() {
			So(provider, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "live capital provider not configured")
		})
	})
}
