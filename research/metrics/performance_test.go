package metrics

import (
	"testing"
	"time"
)

func TestPerformanceCalculatorSummarize(test *testing.T) {
	calculator, calculatorErr := NewPerformanceCalculator(1000)

	if calculatorErr != nil {
		test.Fatalf("calculator: %v", calculatorErr)
	}

	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	trades := []Trade{
		{
			ID:            "trade-1",
			Symbol:        "BTC/USD",
			EntryAt:       base,
			ExitAt:        base.Add(time.Minute),
			EntryNotional: 100,
			ExitNotional:  112,
			Fees:          2,
		},
		{
			ID:            "trade-2",
			Symbol:        "ETH/USD",
			EntryAt:       base.Add(30 * time.Second),
			ExitAt:        base.Add(2 * time.Minute),
			EntryNotional: 200,
			ExitNotional:  180,
			Fees:          1,
		},
	}

	summary, summaryErr := calculator.Summarize(trades)

	if summaryErr != nil {
		test.Fatalf("summary: %v", summaryErr)
	}

	if summary.Trades != 2 {
		test.Fatalf("expected 2 trades, got %d", summary.Trades)
	}

	if summary.Wins != 1 || summary.Losses != 1 {
		test.Fatalf("expected one win and one loss, got %d/%d", summary.Wins, summary.Losses)
	}

	if summary.NetPnL != -11 {
		test.Fatalf("expected net pnl -11, got %f", summary.NetPnL)
	}

	if summary.HitRate != 0.5 {
		test.Fatalf("expected hit rate 0.5, got %f", summary.HitRate)
	}

	if summary.TimeInMarket != 2*time.Minute {
		test.Fatalf("expected merged time in market 2m, got %s", summary.TimeInMarket)
	}
}

func TestPerformanceCalculatorRejectsInvalidTrades(test *testing.T) {
	calculator, calculatorErr := NewPerformanceCalculator(1000)

	if calculatorErr != nil {
		test.Fatalf("calculator: %v", calculatorErr)
	}

	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	_, summaryErr := calculator.Summarize([]Trade{{
		ID:            "bad-trade",
		Symbol:        "BTC/USD",
		EntryAt:       base,
		ExitAt:        base.Add(-time.Second),
		EntryNotional: 100,
		ExitNotional:  90,
	}})

	if summaryErr == nil {
		test.Fatal("expected invalid trade error")
	}
}

func BenchmarkPerformanceCalculatorSummarize(benchmark *testing.B) {
	calculator, calculatorErr := NewPerformanceCalculator(1000)

	if calculatorErr != nil {
		benchmark.Fatal(calculatorErr)
	}

	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	trades := make([]Trade, 0, 128)

	for tradeIndex := 0; tradeIndex < 128; tradeIndex++ {
		entryAt := base.Add(time.Duration(tradeIndex) * time.Minute)
		exitAt := entryAt.Add(30 * time.Second)
		entryNotional := 100.0
		exitNotional := entryNotional + float64((tradeIndex%5)-2)

		trades = append(trades, Trade{
			ID:            "bench",
			Symbol:        "BTC/USD",
			EntryAt:       entryAt,
			ExitAt:        exitAt,
			EntryNotional: entryNotional,
			ExitNotional:  exitNotional,
			Fees:          0.1,
		})
	}

	for benchmark.Loop() {
		if _, summaryErr := calculator.Summarize(trades); summaryErr != nil {
			benchmark.Fatal(summaryErr)
		}
	}
}
