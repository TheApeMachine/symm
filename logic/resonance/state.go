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
		horizonReach: 1,
	}
}
