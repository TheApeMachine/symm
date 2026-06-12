package integration

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/causal"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/manifold"
	"github.com/theapemachine/symm/signal/prediction"
	"github.com/theapemachine/symm/signal/toxicity"
)

func TestIntegratedLeadLagSignalSynchronizedDrift(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return leadlag.NewSystem(ctx, pool)
	})
	base := time.Date(2026, 6, 12, 12, 8, 0, 0, time.UTC)

	for index := range 20 {
		at := base.Add(time.Duration(index) * 15 * time.Second)
		harness.publishTickers(t, []*krakenmarket.TickerUpdate{
			makeTicker("BTC/EUR", 50000+float64(index), 1000, 0.01, at),
			makeTicker("ETH/EUR", 100+float64(index)*2, 1000, 0.01, at),
		})
	}

	harness.awaitMeasurement(
		t,
		"leadlag synchronized drift",
		hasSignalCategory(logic.SourceLeadLag, "ETH/EUR", logic.CategorySynchronizedDrift),
	)
}

func TestIntegratedCausalSignalSystemicBeta(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return causal.NewSystem(ctx, pool)
	})
	base := time.Date(2026, 6, 12, 12, 9, 0, 0, time.UTC)

	harness.publishTickers(t, []*krakenmarket.TickerUpdate{
		makeTicker("BTC/EUR", 50000, 1000, 0.02, base),
	})
	harness.publishBooks(t, []*krakenmarket.BookUpdate{
		makeBook("BTC/EUR", 49990, 10, 50010, 10, base.Add(time.Millisecond)),
	})
	harness.awaitMeasurement(
		t,
		"causal systemic beta",
		hasSignalCategory(logic.SourceCausal, "BTC/EUR", logic.CategorySystemicBeta),
	)
}

func TestIntegratedExhaustSignalMechanicalCollapse(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return exhaust.NewSystem(ctx, pool)
	})
	base := time.Date(2026, 6, 12, 12, 10, 0, 0, time.UTC)
	books := make([]*krakenmarket.BookUpdate, 0, 8)

	for index := range 8 {
		depth := 20.0 - float64(index)*2
		bidPrice := 100 + float64(index)*0.01
		books = append(books, makeBook(
			"XRP/EUR",
			bidPrice,
			depth,
			bidPrice+1,
			depth*0.5,
			base.Add(time.Duration(index)*time.Millisecond),
		))
	}

	for _, book := range books {
		harness.publishBooks(t, []*krakenmarket.BookUpdate{book})
	}

	harness.awaitMeasurement(
		t,
		"exhaust mechanical collapse",
		hasSignalCategory(logic.SourceExhaustion, "XRP/EUR", logic.CategoryMechanicalCollapse),
	)
}

func TestIntegratedToxicitySignalHardSupport(t *testing.T) {
	toxicity.ResetDefault()
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return toxicity.NewSystem(ctx, pool)
	})
	base := time.Date(2026, 6, 12, 12, 11, 0, 0, time.UTC)

	harness.publishTrades(t, []*krakenmarket.TradeUpdate{
		makeTrade("ADA/EUR", "buy", 100, 1, base),
		makeTrade("ADA/EUR", "sell", 100.01, 1, base.Add(time.Millisecond)),
	})
	harness.publishBooks(t, []*krakenmarket.BookUpdate{
		makeBook("ADA/EUR", 99.5, 80, 100.5, 80, base.Add(2*time.Millisecond)),
		makeBook("ADA/EUR", 99.5, 80, 100.5, 80, base.Add(3*time.Millisecond)),
	})
	harness.awaitMeasurement(
		t,
		"toxicity hard support",
		hasSignalCategory(logic.SourceToxicity, "ADA/EUR", logic.CategoryHardSupport),
	)
}

func TestIntegratedManifoldSignalSystemicHerd(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return manifold.NewSystem(ctx, pool)
	})
	base := time.Date(2026, 6, 12, 12, 12, 0, 0, time.UTC)

	harness.publishBooks(t, []*krakenmarket.BookUpdate{
		makeBook("DOT/USD", 99.99, 5, 100.01, 5, base),
		makeBook("DOT/USD", 100.99, 8, 101.01, 8, base.Add(250*time.Millisecond)),
	})
	harness.awaitMeasurement(t, "manifold publishable reading", func(measurement logic.Measurement) bool {
		return measurement.Source == logic.SourceManifold &&
			measurement.Symbol == "DOT/USD" &&
			measurement.Category != logic.CategoryTypeNone &&
			measurement.Publishable()
	})
}

func TestIntegratedPredictionSignalForecast(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return prediction.NewSystem(ctx, pool)
	})
	base := time.Date(2026, 6, 12, 12, 13, 0, 0, time.UTC)
	feature := synthMeasurement(
		logic.SourcePumpDump,
		logic.CategoryVerticalIgnition,
		0.8,
		1.2,
		base,
	)
	feature.Symbol = "AVAX/EUR"

	harness.publishMeasurement(t, feature)
	time.Sleep(50 * time.Millisecond)
	harness.publishTrades(t, []*krakenmarket.TradeUpdate{
		makeTrade("AVAX/EUR", "buy", 100, 1, base),
		makeTrade("AVAX/EUR", "buy", 101, 1, base.Add(time.Millisecond)),
		makeTrade("AVAX/EUR", "buy", 102, 1, base.Add(2*time.Millisecond)),
		makeTrade("AVAX/EUR", "buy", 103, 1, base.Add(3*time.Millisecond)),
	})
	harness.awaitMeasurement(t, "prediction forecast", func(measurement logic.Measurement) bool {
		return measurement.Source == logic.SourcePrediction &&
			measurement.Symbol == "AVAX/EUR" &&
			measurement.Category == logic.CategoryTypeNone &&
			measurement.Publishable()
	})
}
