package hawkes

import (
	"testing"

	"github.com/theapemachine/symm/engine"
)

func TestBeginScanClearsLiveScore(t *testing.T) {
	hawkesSignal := &Hawkes{
		calibrationParams: engine.DefaultCalibrationParams(),
		states: map[string]*HawkesSymbol{
			"PUMP/EUR": {liveScore: 1},
		},
	}

	hawkesSignal.beginScan()

	if hawkesSignal.states["PUMP/EUR"].liveScore != 0 {
		t.Fatalf("expected live score reset, got %v", hawkesSignal.states["PUMP/EUR"].liveScore)
	}
}

func TestPeakLiveConfidenceReflectsCurrentTickOnly(t *testing.T) {
	hawkesSignal := &Hawkes{
		calibrationParams: engine.DefaultCalibrationParams(),
		states: map[string]*HawkesSymbol{
			"AAA/EUR": {liveScore: 1},
		},
	}

	hawkesSignal.beginScan()
	hawkesSignal.state("BBB/EUR").liveScore = 0.25

	if peak := hawkesSignal.LiveScore(); peak != 0.25 {
		t.Fatalf("expected current tick peak 0.25, got %v", peak)
	}
}
