package resonance

import (
	"time"

	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/vector"
)

/*
symbolState holds per-symbol predictive coding and adaptive state, delegating
normalization, error calibration, and learning rate pacing to nomagique primitives.
*/
type symbolState struct {
	manifold      *learning.ResonanceManifold
	alphaCtrl     *learning.PaceController
	confidence    *probability.Calibrator
	featureScale  map[string]*adaptive.Standardizer
	extractor     *vector.FeatureExtractor
	alpha         float64
	featureSchema []string
	pendingInput  []float64
	pendingMid    float64
	pendingAt     time.Time
	rankTrend     *adaptive.EMA
	targetSamples uint64
	horizonReach  int
}

/*
newSymbolState instantiates a fresh symbolState initialized with nomagique primitives.
*/
func newSymbolState(initialAlpha float64) *symbolState {
	return &symbolState{
		alpha: initialAlpha,
		alphaCtrl: learning.NewPaceController(learning.PaceConfig{
			InitialAlpha: initialAlpha,
		}),
		confidence:   probability.NewCalibrator(),
		featureScale: make(map[string]*adaptive.Standardizer),
		rankTrend:    newRankTrend(),
		horizonReach: 1,
	}
}

/*
newRankTrend smooths the pace controller's error rank.

A single rank is uniform by construction and says nothing on its own; what
separates a model that is tracking from one a regime has broken is whether its
errors sit persistently high inside their own recent history.
*/
func newRankTrend() *adaptive.EMA {
	trend, err := adaptive.NewEMA(adaptive.EMAConfig{
		Period:    rankTrendPeriod,
		Smoothing: rankTrendSmoothing,
	})

	if err != nil {
		return nil
	}

	return trend
}

/*
hasFeatures confirms that this observation can populate the fixed input schema
that has already learned return targets. A missing upstream measurement makes
this symbol unavailable for the pass; it must not corrupt another symbol's
logic pass or turn into a synthetic zero.
*/
func (state *symbolState) hasFeatures(features map[string]float64) bool {
	for _, featureKey := range state.featureSchema {
		if _, present := features[featureKey]; !present {
			return false
		}
	}

	return true
}
