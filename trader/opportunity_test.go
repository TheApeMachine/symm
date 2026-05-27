package trader

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestScoreOpportunitiesUsesSingleSignalSupport(t *testing.T) {
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
		returnValue: 0.02,
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

	if summary.Edge <= 0 {
		t.Fatalf("expected fused edge above friction, got %v", summary.Edge)
	}
}

func TestScoreOpportunitiesRecordsButBlocksZeroReturnSupport(t *testing.T) {
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

	if recorder.records != 2 {
		t.Fatalf("expected both predictions recorded, got %d", recorder.records)
	}

	if summary.PredictedCount != 0 {
		t.Fatalf("expected zero-return predictions excluded from telemetry, got %d", summary.PredictedCount)
	}

	if summary.Edge != 0 {
		t.Fatalf("expected missing return support to block entry, got edge %v", summary.Edge)
	}
}

func TestScoreOpportunitiesRequiresEdgeAboveFriction(t *testing.T) {
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

type stubPredictionRecorder struct {
	returnValue float64
	records     int
}

func (recorder *stubPredictionRecorder) Record(
	perspective engine.Perspective,
	measurement engine.Measurement,
	anchorPrice float64,
	now time.Time,
) float64 {
	recorder.records++

	return recorder.returnValue
}
