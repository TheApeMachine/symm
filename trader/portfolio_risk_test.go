package trader

import (
	"math"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func seedSynchronizedPrices(
	portfolioRisk *PortfolioRisk,
	symbol string,
	basePrice float64,
	returns []float64,
	interval time.Duration,
	start time.Time,
) {
	sampleRing := newPriceSampleRing(len(returns) + 1)
	price := basePrice
	sampleRing.push(start, price)

	for index, logReturn := range returns {
		price *= math.Exp(logReturn)
		sampleRing.push(start.Add(time.Duration(index+1)*interval), price)
	}

	portfolioRisk.prices[symbol] = sampleRing
	portfolioRisk.lastPrices[symbol] = price
}

func seedDenseStalePrices(
	portfolioRisk *PortfolioRisk,
	symbol string,
	basePrice float64,
	returns []float64,
	barInterval time.Duration,
	start time.Time,
) {
	tickInterval := time.Second
	capacity := len(returns)*int(barInterval/tickInterval) + len(returns) + 2
	sampleRing := newPriceSampleRing(capacity)
	price := basePrice
	sampleRing.push(start, price)

	for index, logReturn := range returns {
		segmentStart := start.Add(time.Duration(index) * barInterval)

		for tick := segmentStart.Add(tickInterval); tick.Before(segmentStart.Add(barInterval)); tick = tick.Add(tickInterval) {
			sampleRing.push(tick, price)
		}

		price *= math.Exp(logReturn)
		sampleRing.push(segmentStart.Add(barInterval), price)
	}

	portfolioRisk.prices[symbol] = sampleRing
	portfolioRisk.lastPrices[symbol] = price
}

func TestPortfolioRiskBlocksDailyLoss(t *testing.T) {
	portfolioRisk := NewPortfolioRisk()
	now := time.Now()
	portfolioRisk.UpdateEquity(200, now)
	portfolioRisk.dayStartEquity = 200

	wallet := NewWallet(PaperWallet, "EUR", 170, 0.26)
	measurement := engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
		Last:  100,
		Bid:   99.9,
		Ask:   100.1,
	}

	allowed, reason := portfolioRisk.AllowEntry(wallet, measurement, 10, nil)

	if allowed || reason != "daily_loss_limit" {
		t.Fatalf("expected daily loss block, allowed=%v reason=%q", allowed, reason)
	}
}

func TestPortfolioRiskBlocksDrawdown(t *testing.T) {
	portfolioRisk := NewPortfolioRisk()
	now := time.Now()
	portfolioRisk.peakEquity = 200
	portfolioRisk.dayStartEquity = 160
	portfolioRisk.dayAnchor = startOfUTCDate(now)

	wallet := NewWallet(PaperWallet, "EUR", 160, 0.26)
	measurement := engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
		Last:  100,
		Bid:   99.9,
		Ask:   100.1,
	}

	allowed, reason := portfolioRisk.AllowEntry(wallet, measurement, 10, nil)

	if allowed || reason != "drawdown_limit" {
		t.Fatalf("expected drawdown block, allowed=%v reason=%q", allowed, reason)
	}
}

func TestPortfolioRiskBlocksSpread(t *testing.T) {
	original := config.System.MaxSpreadBPS
	config.System.MaxSpreadBPS = 20
	t.Cleanup(func() { config.System.MaxSpreadBPS = original })

	portfolioRisk := NewPortfolioRisk()
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	measurement := engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
		Last:  100,
		Bid:   98,
		Ask:   102,
	}

	allowed, reason := portfolioRisk.AllowEntry(wallet, measurement, 10, nil)

	if allowed || reason[:11] != "spread_bps:" {
		t.Fatalf("expected spread block, allowed=%v reason=%q", allowed, reason)
	}
}

func TestPortfolioRiskBlocksCorrelation(t *testing.T) {
	originalBar := config.System.CorrelationBarSeconds
	config.System.CorrelationBarSeconds = 10
	t.Cleanup(func() { config.System.CorrelationBarSeconds = originalBar })

	portfolioRisk := NewPortfolioRisk()
	values := []float64{0.01, -0.005, 0.002, 0.004, -0.001, 0.003, 0.002, -0.002, 0.001, 0.004, 0.003, 0.002}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval := 10 * time.Second

	seedSynchronizedPrices(portfolioRisk, "BTC/EUR", 100, values, interval, start)
	seedSynchronizedPrices(portfolioRisk, "ETH/EUR", 50, values, interval, start)

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	wallet.Inventory["BTC"] = 0.01
	measurement := engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "ETH/EUR"}},
		Last:  100,
		Bid:   99.9,
		Ask:   100.1,
	}

	allowed, reason := portfolioRisk.AllowEntry(wallet, measurement, 10, []string{"BTC/EUR"})

	if allowed || reason != "correlation_limit" {
		t.Fatalf("expected correlation block, allowed=%v reason=%q", allowed, reason)
	}
}

func TestSymbolCorrelationResampledEpps(t *testing.T) {
	originalBar := config.System.CorrelationBarSeconds
	config.System.CorrelationBarSeconds = 10
	t.Cleanup(func() { config.System.CorrelationBarSeconds = originalBar })

	portfolioRisk := NewPortfolioRisk()
	returns := []float64{
		0.01, -0.005, 0.002, 0.004, -0.001, 0.003,
		0.002, -0.002, 0.001, 0.004, 0.003, 0.002,
		-0.001, 0.002, 0.001, 0.003,
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	barInterval := 10 * time.Second

	seedSynchronizedPrices(portfolioRisk, "GAIA/EUR", 10, returns, barInterval, start)
	seedDenseStalePrices(portfolioRisk, "BTC/EUR", 100, returns, barInterval, start)

	correlation, ok := portfolioRisk.symbolCorrelation("BTC/EUR", "GAIA/EUR")

	if !ok {
		t.Fatal("expected resampled correlation to succeed")
	}

	if math.Abs(correlation-1) > 1e-6 {
		t.Fatalf("expected resampled correlation ~1, got %v", correlation)
	}
}

func TestPortfolioRiskBlocksExistingSymbol(t *testing.T) {
	portfolioRisk := NewPortfolioRisk()
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	wallet.Inventory["BTC"] = 0.01
	measurement := engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
		Last:  100,
		Bid:   99.9,
		Ask:   100.1,
	}

	allowed, reason := portfolioRisk.AllowEntry(wallet, measurement, 10, []string{"BTC/EUR"})

	if allowed || reason != "symbol_already_open" {
		t.Fatalf("expected existing symbol block, allowed=%v reason=%q", allowed, reason)
	}
}

func TestPortfolioRiskBlocksDeployLimit(t *testing.T) {
	original := config.System.MaxDeployPct
	config.System.MaxDeployPct = 10
	t.Cleanup(func() { config.System.MaxDeployPct = original })

	portfolioRisk := NewPortfolioRisk()
	portfolioRisk.lastPrices["BTC/EUR"] = 100
	portfolioRisk.UpdateEquity(200, time.Now())

	wallet := NewWallet(PaperWallet, "EUR", 150, 0.26)
	wallet.Inventory["BTC"] = 0.4
	measurement := engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "ETH/EUR"}},
		Last:  100,
		Bid:   99.9,
		Ask:   100.1,
	}

	allowed, reason := portfolioRisk.AllowEntry(wallet, measurement, 20, []string{"BTC/EUR"})

	if allowed || reason != "deploy_limit" {
		t.Fatalf("expected deploy block, allowed=%v reason=%q", allowed, reason)
	}
}

func TestPearsonPerfectCorrelation(t *testing.T) {
	left := []float64{0.01, -0.02, 0.03, 0.01}
	right := []float64{0.02, -0.04, 0.06, 0.02}
	correlation := pearson(left, right)

	if math.Abs(correlation-1) > 1e-9 {
		t.Fatalf("expected correlation 1, got %v", correlation)
	}
}

func TestWalletMarkEquity(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 100, 0.26)
	wallet.ReservedEUR = 20
	wallet.Inventory["BTC"] = 0.5

	equity := wallet.MarkEquity(map[string]float64{"BTC/EUR": 200})

	if math.Abs(equity-220) > 1e-9 {
		t.Fatalf("expected equity 220, got %v", equity)
	}
}

func BenchmarkPortfolioRiskAllowEntry(b *testing.B) {
	portfolioRisk := NewPortfolioRisk()
	values := []float64{0.01, -0.005, 0.002, 0.004, -0.001, 0.003, 0.002, -0.002, 0.001, 0.004, 0.003, 0.002}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval := 10 * time.Second

	seedSynchronizedPrices(portfolioRisk, "BTC/EUR", 100, values, interval, start)
	seedSynchronizedPrices(portfolioRisk, "ETH/EUR", 50, values, interval, start)
	portfolioRisk.UpdateEquity(200, time.Now())

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	measurement := engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "ETH/EUR"}},
		Last:  100,
		Bid:   99.9,
		Ask:   100.1,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = portfolioRisk.AllowEntry(wallet, measurement, 10, []string{"BTC/EUR"})
	}
}

func BenchmarkSymbolCorrelationResampled(b *testing.B) {
	portfolioRisk := NewPortfolioRisk()
	returns := []float64{
		0.01, -0.005, 0.002, 0.004, -0.001, 0.003,
		0.002, -0.002, 0.001, 0.004, 0.003, 0.002,
		-0.001, 0.002, 0.001,
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	seedSynchronizedPrices(portfolioRisk, "BTC/EUR", 100, returns, time.Second, start)

	gaiaRing := newPriceSampleRing(len(returns) + 1)
	price := 10.0
	gaiaRing.push(start, price)

	for index, logReturn := range returns {
		price *= math.Exp(logReturn)
		gaiaRing.push(start.Add(time.Duration((index+1)*10)*time.Second), price)
	}

	portfolioRisk.prices["GAIA/EUR"] = gaiaRing

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = portfolioRisk.symbolCorrelation("BTC/EUR", "GAIA/EUR")
	}
}
