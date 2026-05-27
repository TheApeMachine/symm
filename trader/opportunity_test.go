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

	recorder := &stubPredictionRecorder{
		returnValue: 0.02,
		calibrated:  map[string]bool{"pumpdump": true},
	}
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

	recorder := &stubPredictionRecorder{
		returnValue: 0.02,
		calibrated: map[string]bool{
			"pumpdump": true,
			"hawkes":   true,
		},
	}
	summary := scoreOpportunities(recorder, perspectives, now)

	if summary.Opportunity.SourceCount < config.System.MinActivePerspectives {
		t.Fatalf("expected fused sources, got %d", summary.Opportunity.SourceCount)
	}

	if summary.Edge <= config.System.MinEdgeReturn {
		t.Fatalf("expected fused edge above hurdle, got %v", summary.Edge)
	}
}

func TestScoreOpportunitiesRecordsButBlocksUncalibratedSources(t *testing.T) {
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

	recorder := &stubPredictionRecorder{returnValue: 0.42}
	summary := scoreOpportunities(recorder, perspectives, now)

	if recorder.records != 2 {
		t.Fatalf("expected both uncalibrated predictions recorded, got %d", recorder.records)
	}

	if summary.PredictedCount != 2 {
		t.Fatalf("expected predictions counted for telemetry, got %d", summary.PredictedCount)
	}

	if summary.Edge != 0 {
		t.Fatalf("expected uncalibrated sources blocked from entry, got edge %v", summary.Edge)
	}
}

type stubPredictionRecorder struct {
	returnValue float64
	calibrated  map[string]bool
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

func (recorder *stubPredictionRecorder) Calibrated(source string) bool {
	return recorder.calibrated[source]
}
