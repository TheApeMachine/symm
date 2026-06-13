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
		logic.Measurement{
			Source:          logic.SourceHawkes,
			Symbol:          "BTC/USD",
			Price:           50_000,
			Strength:        1,
			Volume:          1,
			Spread:          1,
			Elapsed:         1,
			Category:        logic.CategoryFrenzy,
			Confidence:      confidence,
			Surprise:        surprise,
			ExpectedMoveBps: 80,
			EdgeConfidence:  confidence,
			Position:        logic.PositionTypeLong,
			DecisionGrade:   logic.DecisionGradeExecutable,
		},
		logic.Measurement{
			Source:          logic.SourceCVD,
			Symbol:          "BTC/USD",
			Price:           50_000,
			Strength:        0.8,
			Volume:          1,
			Spread:          1,
			Elapsed:         1,
			Category:        logic.CategoryAggressiveDrive,
			Confidence:      confidence * 0.9,
			Surprise:        surprise * 0.8,
			ExpectedMoveBps: 80,
			EdgeConfidence:  confidence * 0.9,
			Position:        logic.PositionTypeLong,
			DecisionGrade:   logic.DecisionGradeExecutable,
		},
	}
}

func sampleCandidate(confidence float64, surprise float64, strength float64) logic.EntryCandidate {
	return logic.EntryCandidate{
		Sources:           []logic.SourceType{logic.SourceHawkes},
		Categories:        []logic.CategoryType{logic.CategoryFrenzy},
		ExpectedDirection: logic.PositionTypeLong,
		Confidence:        confidence,
		EdgeBps:           40,
		CostBps:           20,
		Strength:          strength,
		Novelty:           surprise,
	}
}

func TestEntrySlotFraction(test *testing.T) {
	tradingConfig := config.TradingConfig{
		MaxConcurrentPositions: 4,
		OpportunitySlotCount:   2,
	}
	thresholdCtx := logic.NewThresholdContext(0.55, 0, 0)

	Convey("Given a six-slot envelope and a strong measurement spectrum", test, func() {
		holdings := logic.NewHoldings()
		measurements := sampleMeasurements(0.9, 1.2)
		executionCost := logic.ExecutionCostFromMarket(measurements, 0, 0, 0)
		candidate, candidateOK := logic.BestEntryCandidate(measurements, executionCost)

		So(candidateOK, ShouldBeTrue)

		fraction, err := EntrySlotFraction(
			holdings,
			logic.EntrySlotOccupancyFromHoldings(holdings),
			measurements,
			executionCost,
			candidate,
			thresholdCtx,
			tradingConfig,
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
		strongCost := logic.ExecutionCostFromMarket(strongMeasurements, 0, 0, 0)
		weakCost := logic.ExecutionCostFromMarket(weakMeasurements, 0, 0, 0)
		strongCandidate, strongCandidateOK := logic.BestEntryCandidate(strongMeasurements, strongCost)
		weakCandidate, weakCandidateOK := logic.BestEntryCandidate(weakMeasurements, weakCost)

		So(strongCandidateOK, ShouldBeTrue)
		So(weakCandidateOK, ShouldBeTrue)

		strongFraction, strongErr := EntrySlotFraction(
			holdings,
			logic.EntrySlotOccupancyFromHoldings(holdings),
			strongMeasurements,
			strongCost,
			strongCandidate,
			thresholdCtx,
			tradingConfig,
			false,
		)
		weakFraction, weakErr := EntrySlotFraction(
			holdings,
			logic.EntrySlotOccupancyFromHoldings(holdings),
			weakMeasurements,
			weakCost,
			weakCandidate,
			thresholdCtx,
			tradingConfig,
			false,
		)

		Convey("It should size stronger evidence larger than weaker evidence", func() {
			So(strongErr, ShouldBeNil)
			So(weakErr, ShouldBeNil)
			So(strongFraction, ShouldBeGreaterThan, weakFraction)
		})
	})

	Convey("Given unequal candidate strengths with the same confidence", test, func() {
		holdings := logic.NewHoldings()
		measurements := sampleMeasurements(0.85, 1.0)
		executionCost := logic.ExecutionCostFromMarket(measurements, 0, 0, 0)

		strongFraction, strongErr := EntrySlotFraction(
			holdings,
			logic.EntrySlotOccupancyFromHoldings(holdings),
			measurements,
			executionCost,
			sampleCandidate(0.85, 1.0, 1.2),
			thresholdCtx,
			tradingConfig,
			false,
		)
		weakFraction, weakErr := EntrySlotFraction(
			holdings,
			logic.EntrySlotOccupancyFromHoldings(holdings),
			measurements,
			executionCost,
			sampleCandidate(0.85, 1.0, 0.4),
			thresholdCtx,
			tradingConfig,
			false,
		)

		Convey("It should weight stronger candidates larger", func() {
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

		primaryHoldings := logic.NewHoldings()
		measurements := sampleMeasurements(0.85, 1.0)
		executionCost := logic.ExecutionCostFromMarket(measurements, 0, 0, 0)
		candidate, candidateOK := logic.BestEntryCandidate(measurements, executionCost)

		So(candidateOK, ShouldBeTrue)

		primaryFraction, primaryErr := EntrySlotFraction(
			primaryHoldings,
			logic.EntrySlotOccupancyFromHoldings(primaryHoldings),
			measurements,
			executionCost,
			candidate,
			thresholdCtx,
			tradingConfig,
			false,
		)
		secondaryFraction, secondaryErr := EntrySlotFraction(
			holdings,
			logic.EntrySlotOccupancyFromHoldings(holdings),
			measurements,
			executionCost,
			candidate,
			thresholdCtx,
			tradingConfig,
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
	thresholdCtx := logic.NewThresholdContext(0.55, 0, 0)
	executionCost := logic.ExecutionCostFromMarket(measurements, 0, 0, 0)
	candidate, _ := logic.BestEntryCandidate(measurements, executionCost)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = EntrySlotFraction(
			holdings,
			logic.EntrySlotOccupancyFromHoldings(holdings),
			measurements,
			executionCost,
			candidate,
			thresholdCtx,
			tradingConfig,
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

	Convey("Given live trading with wallet-backed capital", t, func() {
		provider, err := NewCapitalProvider(config.TradingConfig{Model: "live"})

		Convey("It should provide a wallet capital provider", func() {
			So(err, ShouldBeNil)
			So(provider, ShouldNotBeNil)
			_, ok := provider.(*WalletCapitalProvider)
			So(ok, ShouldBeTrue)
		})
	})
}
