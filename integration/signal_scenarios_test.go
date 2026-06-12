package integration

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
)

func TestIntegratedCVDSignalAggressiveDrive(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return cvd.NewSystem(ctx, pool)
	})
	base := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	trades := make([]*krakenmarket.TradeUpdate, 0, 6)

	for index := range 6 {
		trades = append(trades, makeTrade(
			"BTC/EUR",
			"buy",
			100+float64(index),
			1,
			base.Add(time.Duration(index)*time.Millisecond),
		))
	}

	harness.publishTrades(t, trades)
	harness.awaitMeasurement(
		t,
		"cvd aggressive drive",
		hasSignalCategory(logic.SourceCVD, "BTC/EUR", logic.CategoryAggressiveDrive),
	)
}

func TestIntegratedPumpDumpSignalVerticalIgnition(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return pumpdump.NewSystem(ctx, pool)
	})
	base := time.Date(2026, 6, 12, 12, 1, 0, 0, time.UTC)
	trades := []*krakenmarket.TradeUpdate{
		makeTrade("ETH/EUR", "buy", 100, 1, base),
		makeTrade("ETH/EUR", "buy", 101, 1, base.Add(time.Millisecond)),
		makeTrade("ETH/EUR", "buy", 102, 1, base.Add(2*time.Millisecond)),
		makeTrade("ETH/EUR", "buy", 103, 1, base.Add(3*time.Millisecond)),
		makeTrade("ETH/EUR", "buy", 108, 25, base.Add(4*time.Millisecond)),
		makeTrade("ETH/EUR", "buy", 115, 30, base.Add(5*time.Millisecond)),
		makeTrade("ETH/EUR", "buy", 123, 35, base.Add(6*time.Millisecond)),
	}

	harness.publishTrades(t, trades)
	harness.awaitMeasurement(
		t,
		"pumpdump vertical ignition",
		hasSignalCategory(logic.SourcePumpDump, "ETH/EUR", logic.CategoryVerticalIgnition),
	)
}

func TestIntegratedLiquiditySignalExtremeScarcity(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return liquidity.NewSystem(ctx, pool)
	})
	at := time.Date(2026, 6, 12, 12, 2, 0, 0, time.UTC)

	harness.publishTickers(t, []*krakenmarket.TickerUpdate{
		makeTicker("DEEP/EUR", 10, 1100, 0.01, at),
		makeTicker("MID/EUR", 10, 950, 0.01, at),
	})
	time.Sleep(100 * time.Millisecond)
	harness.publishTickers(t, []*krakenmarket.TickerUpdate{
		makeTicker("THIN/EUR", 5, 50, 0.01, at.Add(time.Millisecond)),
	})
	harness.awaitMeasurement(
		t,
		"liquidity extreme scarcity",
		hasSignalCategory(logic.SourceLiquidity, "THIN/EUR", logic.CategoryExtremeScarcity),
	)
}

func TestIntegratedSentimentSignalRiskOnSurge(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return sentiment.NewSystem(ctx, pool)
	})
	at := time.Date(2026, 6, 12, 12, 3, 0, 0, time.UTC)

	harness.publishTickers(t, []*krakenmarket.TickerUpdate{
		makeTicker("A/EUR", 104, 1000, 2.0, at),
		makeTicker("B/EUR", 203, 1000, 2.0, at),
		makeTicker("C/EUR", 302, 1000, 2.0, at),
	})
	harness.awaitMeasurement(
		t,
		"sentiment risk-on surge",
		hasSignalCategory(logic.SourceSentiment, "A/EUR", logic.CategoryRiskOnSurge),
	)
}

func TestIntegratedDepthFlowSignalSpoofTrap(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return depthflow.NewSystem(ctx, pool)
	})
	base := time.Date(2026, 6, 12, 12, 4, 0, 0, time.UTC)

	harness.publishTrades(t, []*krakenmarket.TradeUpdate{
		makeTrade("ETH/EUR", "sell", 50, 1, base),
		makeTrade("ETH/EUR", "sell", 49.9, 1, base.Add(time.Millisecond)),
	})

	harness.publishBooks(t, []*krakenmarket.BookUpdate{
		{
			Symbol:    "ETH/EUR",
			Type:      "snapshot",
			Timestamp: base.Add(2 * time.Millisecond),
			Bids: []krakenmarket.BookLevel{
				{Price: 49, Qty: 1},
				{Price: 48, Qty: 30},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 51, Qty: 8},
				{Price: 52, Qty: 8},
			},
		},
		{
			Symbol:    "ETH/EUR",
			Type:      "snapshot",
			Timestamp: base.Add(3 * time.Millisecond),
			Bids: []krakenmarket.BookLevel{
				{Price: 49, Qty: 2},
				{Price: 48, Qty: 30},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 51, Qty: 8},
				{Price: 52, Qty: 8},
			},
		},
		{
			Symbol:    "ETH/EUR",
			Type:      "snapshot",
			Timestamp: base.Add(4 * time.Millisecond),
			Bids: []krakenmarket.BookLevel{
				{Price: 49, Qty: 2},
				{Price: 48, Qty: 30},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 51, Qty: 8},
				{Price: 52, Qty: 8},
			},
		},
		{
			Symbol:    "ETH/EUR",
			Type:      "snapshot",
			Timestamp: base.Add(5 * time.Millisecond),
			Bids: []krakenmarket.BookLevel{
				{Price: 49, Qty: 2},
				{Price: 48, Qty: 30},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 51, Qty: 8},
				{Price: 52, Qty: 8},
			},
		},
	})
	harness.awaitMeasurement(
		t,
		"depthflow spoof trap",
		hasSignalCategory(logic.SourceDepthFlow, "ETH/EUR", logic.CategorySpoofTrap),
	)
}

func TestIntegratedCorrelationSignalSystemicHerd(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return correlation.NewSystem(ctx, pool)
	})
	base := time.Date(2026, 6, 12, 12, 5, 0, 0, time.UTC)

	for index := range 4 {
		at := base.Add(time.Duration(index) * time.Second)
		harness.publishTickers(t, []*krakenmarket.TickerUpdate{
			makeTicker("AAA/EUR", 100+float64(index), 1000, float64(index+1)*0.01, at),
			makeTicker("BBB/EUR", 200+float64(index)*2, 1000, float64(index+1)*0.01, at),
			makeTicker("CCC/EUR", 300+float64(index)*3, 1000, float64(index+1)*0.01, at),
		})
	}

	harness.awaitMeasurement(
		t,
		"correlation systemic herd",
		func(measurement logic.Measurement) bool {
			return measurement.Source == logic.SourceCorrelation &&
				measurement.Category == logic.CategorySystemicHerd &&
				measurement.Publishable()
		},
	)
}

func TestIntegratedFluidSignalInertial(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return fluid.NewSystem(ctx, pool)
	})
	base := time.Date(2026, 6, 12, 12, 6, 0, 0, time.UTC)

	harness.publishTickers(t, []*krakenmarket.TickerUpdate{
		makeTicker("BTC/EUR", 100, 1000, 0.01, base),
	})
	harness.publishBooks(t, []*krakenmarket.BookUpdate{
		makeBook("BTC/EUR", 99.99, 5, 100.01, 5, base.Add(100*time.Millisecond)),
		makeBook("BTC/EUR", 99.99, 8, 100.01, 8, base.Add(250*time.Millisecond)),
	})
	harness.awaitMeasurement(
		t,
		"fluid inertial field",
		hasSignalCategory(logic.SourceFluid, "BTC/EUR", logic.CategoryInertial),
	)
}

func TestIntegratedHawkesSignalPublishesClusterReading(t *testing.T) {
	harness := newSignalScenarioHarness(t, func(ctx context.Context, pool *qpool.Q[any]) signalRunner {
		return hawkes.NewSystem(ctx, pool)
	})
	base := time.Date(2026, 6, 12, 12, 7, 0, 0, time.UTC)
	trades := make([]*krakenmarket.TradeUpdate, 0, 128)

	for index := range 128 {
		side := "sell"

		if index%2 == 1 {
			side = "buy"
		}

		trades = append(trades, makeTrade(
			"ALT/EUR",
			side,
			100+float64(index)*0.01,
			1.5+float64(index%5)*0.1,
			base.Add(time.Duration(index)*100*time.Millisecond),
		))
	}

	harness.publishTrades(t, trades)
	harness.awaitMeasurement(t, "hawkes cluster reading", func(measurement logic.Measurement) bool {
		return measurement.Source == logic.SourceHawkes &&
			measurement.Symbol == "ALT/EUR" &&
			measurement.Category != logic.CategoryTypeNone &&
			measurement.Publishable()
	})
}
