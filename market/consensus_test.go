package market

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

func TestConsensusEntryAction(t *testing.T) {
	observedAt := time.Now()
	measurements := []logic.Measurement{
		{
			Source:     logic.SourceSentiment,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryRiskOnSurge,
			Confidence: 0.8,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceLiquidity,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryExtremeScarcity,
			Confidence: 0.7,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceDepthFlow,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategorySpoofTrap,
			Confidence: 0.3,
			ObservedAt: observedAt,
		},
	}

	action, err := newConsensusEntry(measurements).Action(
		measurements,
		logic.NewHoldings(),
	)

	if err != nil {
		t.Fatalf("consensus action: %v", err)
	}

	if action == nil {
		t.Fatalf("expected consensus entry action")
	}

	if action.Type != logic.ActionMarket {
		t.Fatalf("expected market action, got %q", action.Type)
	}

	if action.Side != trading.Buy {
		t.Fatalf("expected buy action, got %q", action.Side)
	}

	if action.Symbol != "BTC/USD" {
		t.Fatalf("expected BTC/USD symbol, got %q", action.Symbol)
	}
}

func TestConsensusEntryActionRejectsRiskDominance(t *testing.T) {
	observedAt := time.Now()
	measurements := []logic.Measurement{
		{
			Source:     logic.SourceSentiment,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryRiskOnSurge,
			Confidence: 0.4,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceDepthFlow,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategorySpoofTrap,
			Confidence: 0.8,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceToxicity,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryToxicBluff,
			Confidence: 0.7,
			ObservedAt: observedAt,
		},
	}

	action, err := newConsensusEntry(measurements).Action(
		measurements,
		logic.NewHoldings(),
	)

	if err != nil {
		t.Fatalf("consensus action: %v", err)
	}

	if action != nil {
		t.Fatalf("expected no action, got %#v", action)
	}
}

func BenchmarkConsensusEntryAction(benchmark *testing.B) {
	observedAt := time.Now()
	measurements := []logic.Measurement{
		{
			Source:     logic.SourceSentiment,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryRiskOnSurge,
			Confidence: 0.8,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceLiquidity,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryExtremeScarcity,
			Confidence: 0.7,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceDepthFlow,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategorySpoofTrap,
			Confidence: 0.3,
			ObservedAt: observedAt,
		},
	}
	holdings := logic.NewHoldings()

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for range benchmark.N {
		if _, err := newConsensusEntry(measurements).Action(
			measurements,
			holdings,
		); err != nil {
			benchmark.Fatal(err)
		}
	}
}
