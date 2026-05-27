package trader

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestScoreOpportunitiesUsesSingleSignalSupport(t *testing.T) {
	withOpportunityFees(t)

	now := time.Now()
	perspectives := map[string]map[engine.PerspectiveType]engine.Perspective{
		"BTC/EUR": {
			engine.PerspectiveMicrostructure: {
				Type: engine.PerspectiveMicrostructure,
				Measurements: []engine.Measurement{
					{
						Source:     "pumpdump",
						Type:       engine.Pump,
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.9,
						Last:       100,
					},
				},
			},
		},
	}

	recorder := &stubPredictionRecorder{
		returnValue: 0.005,
	}
	summary := scoreOpportunities(recorder, perspectives, now)

	if summary.Edge <= 0 {
		t.Fatal("expected supported single signal to produce opportunity")
	}

	if summary.Opportunity.SourceCount != 1 {
		t.Fatalf("expected one supporting source, got %d", summary.Opportunity.SourceCount)
	}
}

func TestScoreOpportunitiesFusesSources(t *testing.T) {
	withOpportunityFees(t)

	now := time.Now()
	perspectives := map[string]map[engine.PerspectiveType]engine.Perspective{
		"BTC/EUR": {
			engine.PerspectiveMicrostructure: {
				Type: engine.PerspectiveMicrostructure,
				Measurements: []engine.Measurement{
					{
						Source:     "pumpdump",
						Type:       engine.Pump,
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.8,
						Last:       100,
					},
					{
						Source:     "hawkes",
						Type:       engine.Momentum,
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.7,
						Last:       100,
					},
				},
			},
		},
	}

	recorder := &stubPredictionRecorder{
		returnValue: 0.02,
	}
	summary := scoreOpportunities(recorder, perspectives, now)

	if summary.Opportunity.SourceCount != 2 {
		t.Fatalf("expected fused sources, got %d", summary.Opportunity.SourceCount)
	}

	if summary.Opportunity.JointConfidence <= 0.7 {
		t.Fatalf("expected source confidence fusion without re-squashing, got %v", summary.Opportunity.JointConfidence)
	}

	if summary.Edge <= 0 {
		t.Fatalf("expected fused edge above friction, got %v", summary.Edge)
	}
}

func TestScoreOpportunitiesUsesPerspectivePrediction(t *testing.T) {
	withOpportunityFees(t)

	now := time.Now()
	perspectives := map[string]map[engine.PerspectiveType]engine.Perspective{
		"BTC/EUR": {
			engine.PerspectiveMicrostructure: {
				Type: engine.PerspectiveMicrostructure,
				Measurements: []engine.Measurement{
					{
						Source:     "pumpdump",
						Type:       engine.Pump,
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.9,
						Last:       100,
					},
					{
						Source:     "hawkes",
						Type:       engine.Momentum,
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.1,
						Last:       100,
					},
				},
			},
		},
	}

	recorder := &stubPredictionRecorder{returnValue: 0.009}
	summary := scoreOpportunities(recorder, perspectives, now)

	if recorder.records != 1 {
		t.Fatalf("expected one perspective prediction, got %d", recorder.records)
	}

	if summary.Opportunity.SourceCount != 2 {
		t.Fatalf("expected two supporting sources, got %d", summary.Opportunity.SourceCount)
	}

	if summary.Opportunity.PredictedReturn != 0.009 {
		t.Fatalf("expected perspective forecast, got %v", summary.Opportunity.PredictedReturn)
	}

	if summary.Opportunity.PredictedReturn <= summary.Opportunity.Friction {
		t.Fatalf("expected perspective return above friction, got %+v", summary.Opportunity)
	}
}

func TestScoreOpportunitiesRecordsButBlocksZeroReturnSupport(t *testing.T) {
	withOpportunityFees(t)

	now := time.Now()
	perspectives := map[string]map[engine.PerspectiveType]engine.Perspective{
		"BTC/EUR": {
			engine.PerspectiveMicrostructure: {
				Type: engine.PerspectiveMicrostructure,
				Measurements: []engine.Measurement{
					{
						Source:     "pumpdump",
						Type:       engine.Pump,
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.8,
						Last:       100,
					},
					{
						Source:     "hawkes",
						Type:       engine.Momentum,
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.7,
						Last:       100,
					},
				},
			},
		},
	}

	recorder := &stubPredictionRecorder{}
	summary := scoreOpportunities(recorder, perspectives, now)

	if recorder.records != 1 {
		t.Fatalf("expected one perspective prediction recorded, got %d", recorder.records)
	}

	if summary.PredictedCount != 0 {
		t.Fatalf("expected zero-return predictions excluded from telemetry, got %d", summary.PredictedCount)
	}

	if summary.Edge != 0 {
		t.Fatalf("expected missing return support to block entry, got edge %v", summary.Edge)
	}
}

func TestScoreOpportunitiesRequiresEdgeAboveFriction(t *testing.T) {
	withOpportunityFees(t)

	now := time.Now()
	perspectives := map[string]map[engine.PerspectiveType]engine.Perspective{
		"BTC/EUR": {
			engine.PerspectiveMicrostructure: {
				Type: engine.PerspectiveMicrostructure,
				Measurements: []engine.Measurement{
					{
						Source:     "pumpdump",
						Type:       engine.Pump,
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.8,
						Last:       100,
					},
					{
						Source:     "hawkes",
						Type:       engine.Momentum,
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.7,
						Last:       100,
					},
				},
			},
		},
	}

	recorder := &stubPredictionRecorder{
		returnValue: 0.001,
	}
	summary := scoreOpportunities(recorder, perspectives, now)

	if summary.Edge != 0 {
		t.Fatalf("expected friction to block weak edge, got edge %v", summary.Edge)
	}
}

func BenchmarkScoreOpportunities(b *testing.B) {
	originalUseMakerEntries := config.System.UseMakerEntries
	originalMakerFeePct := config.System.MakerFeePct
	originalTakerFeePct := config.System.TakerFeePct
	config.System.UseMakerEntries = true
	config.System.MakerFeePct = 0.16
	config.System.TakerFeePct = 0.26
	defer func() {
		config.System.UseMakerEntries = originalUseMakerEntries
		config.System.MakerFeePct = originalMakerFeePct
		config.System.TakerFeePct = originalTakerFeePct
	}()

	now := time.Now()
	perspectives := map[string]map[engine.PerspectiveType]engine.Perspective{
		"BTC/EUR": {
			engine.PerspectiveMicrostructure: {
				Type: engine.PerspectiveMicrostructure,
				Measurements: []engine.Measurement{
					{
						Source:     "pumpdump",
						Type:       engine.Pump,
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.8,
						Last:       100,
						Bid:        99.99,
						Ask:        100.01,
					},
					{
						Source:     "hawkes",
						Type:       engine.Momentum,
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.7,
						Last:       100,
						Bid:        99.99,
						Ask:        100.01,
					},
				},
			},
		},
	}
	recorder := &stubPredictionRecorder{
		returnValue: 0.02,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = scoreOpportunities(recorder, perspectives, now)
	}
}

func withOpportunityFees(t *testing.T) {
	t.Helper()

	originalUseMakerEntries := config.System.UseMakerEntries
	originalMakerFeePct := config.System.MakerFeePct
	originalTakerFeePct := config.System.TakerFeePct
	config.System.UseMakerEntries = true
	config.System.MakerFeePct = 0.16
	config.System.TakerFeePct = 0.26

	t.Cleanup(func() {
		config.System.UseMakerEntries = originalUseMakerEntries
		config.System.MakerFeePct = originalMakerFeePct
		config.System.TakerFeePct = originalTakerFeePct
	})
}

type stubPredictionRecorder struct {
	returnValue float64
	records     int
}

func (recorder *stubPredictionRecorder) RecordPerspective(
	symbol string,
	perspective engine.Perspective,
	now time.Time,
) float64 {
	recorder.records++

	return recorder.returnValue
}
