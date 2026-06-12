package market

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

var consensusThresholdFixture = config.ThresholdConfig{
	EntryConfidenceBaseline: 0.55,
	EntrySurpriseBaseline:   1.0,
}

func TestConsensusEntryAction(t *testing.T) {
	observedAt := time.Now()
	measurements := []logic.Measurement{
		{
			Source:     logic.SourceSentiment,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryRiskOnSurge,
			Confidence: 0.8,
			Surprise:   2,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceLiquidity,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryExtremeScarcity,
			Confidence: 0.7,
			Surprise:   2,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceHawkes,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryOrganic,
			Confidence: 0.7,
			Surprise:   2,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceDepthFlow,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategorySpoofTrap,
			Confidence: 0.3,
			Surprise:   2,
			ObservedAt: observedAt,
		},
	}

	action, err := newConsensusEntry(
		measurements,
		consensusThresholdFixture,
	).Action(
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

func TestConsensusEntryActionRejectsMacroOnlyVotes(t *testing.T) {
	observedAt := time.Now()
	measurements := []logic.Measurement{
		{
			Source:     logic.SourceSentiment,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryRiskOnSurge,
			Confidence: 0.8,
			Surprise:   2,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceManifold,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategorySystemicHerd,
			Confidence: 0.7,
			Surprise:   2,
			ObservedAt: observedAt,
		},
	}

	action, err := newConsensusEntry(
		measurements,
		consensusThresholdFixture,
	).Action(
		measurements,
		logic.NewHoldings(),
	)

	if err != nil {
		t.Fatalf("consensus action: %v", err)
	}

	if action != nil {
		t.Fatalf("expected no macro-only action, got %#v", action)
	}
}

func TestConsensusEntryActionRejectsMacroRiskContext(t *testing.T) {
	observedAt := time.Now()
	measurements := []logic.Measurement{
		{
			Source:     logic.SourceCausal,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategorySystemicBeta,
			Confidence: 0.9,
			Surprise:   2,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceHawkes,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryOrganic,
			Confidence: 0.8,
			Surprise:   2,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceDepthFlow,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategorySpoofTrap,
			Confidence: 0.8,
			Surprise:   2,
			ObservedAt: observedAt,
		},
	}

	action, err := newConsensusEntry(
		measurements,
		consensusThresholdFixture,
	).Action(
		measurements,
		logic.NewHoldings(),
	)

	if err != nil {
		t.Fatalf("consensus action: %v", err)
	}

	if action != nil {
		t.Fatalf("expected macro risk gate to reject entry, got %#v", action)
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
			Surprise:   2,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceDepthFlow,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategorySpoofTrap,
			Confidence: 0.8,
			Surprise:   2,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceToxicity,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryToxicBluff,
			Confidence: 0.7,
			Surprise:   2,
			ObservedAt: observedAt,
		},
	}

	action, err := newConsensusEntry(
		measurements,
		consensusThresholdFixture,
	).Action(
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

func TestConsensusEntryActionRejectsWeakSignalFloor(t *testing.T) {
	observedAt := time.Now()
	measurements := []logic.Measurement{
		{
			Source:     logic.SourceSentiment,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryRiskOnSurge,
			Confidence: 0.4,
			Surprise:   2,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceLiquidity,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryExtremeScarcity,
			Confidence: 0.45,
			Surprise:   2,
			ObservedAt: observedAt,
		},
	}

	action, err := newConsensusEntry(
		measurements,
		consensusThresholdFixture,
	).Action(
		measurements,
		logic.NewHoldings(),
	)

	if err != nil {
		t.Fatalf("consensus action: %v", err)
	}

	if action != nil {
		t.Fatalf("expected no action below configured floor, got %#v", action)
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
			Surprise:   2,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceLiquidity,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategoryExtremeScarcity,
			Confidence: 0.7,
			Surprise:   2,
			ObservedAt: observedAt,
		},
		{
			Source:     logic.SourceDepthFlow,
			Symbol:     "BTC/USD",
			Price:      100,
			Category:   logic.CategorySpoofTrap,
			Confidence: 0.3,
			Surprise:   2,
			ObservedAt: observedAt,
		},
	}
	holdings := logic.NewHoldings()

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for range benchmark.N {
		if _, err := newConsensusEntry(
			measurements,
			consensusThresholdFixture,
		).Action(
			measurements,
			holdings,
		); err != nil {
			benchmark.Fatal(err)
		}
	}
}
