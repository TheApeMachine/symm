package trader

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestScoreOpportunitiesRequiresFusion(t *testing.T) {
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

	recorder := &stubPredictionRecorder{returnValue: 0.02}
	summary := scoreOpportunities(recorder, perspectives, now)

	if summary.Edge > 0 {
		t.Fatal("expected single-source perspective to be rejected")
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

	recorder := &stubPredictionRecorder{returnValue: 0.02}
	summary := scoreOpportunities(recorder, perspectives, now)

	if summary.Opportunity.SourceCount < config.System.MinActivePerspectives {
		t.Fatalf("expected fused sources, got %d", summary.Opportunity.SourceCount)
	}

	if summary.Edge <= config.System.MinEdgeReturn {
		t.Fatalf("expected fused edge above hurdle, got %v", summary.Edge)
	}
}

type stubPredictionRecorder struct {
	returnValue float64
}

func (recorder *stubPredictionRecorder) Record(
	perspective engine.Perspective,
	measurement engine.Measurement,
	anchorPrice float64,
	now time.Time,
) float64 {
	return recorder.returnValue
}
