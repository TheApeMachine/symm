package resonance

import (
	"time"

	"github.com/theapemachine/nomagique/learning"
)

type symbolState struct {
	manifold      *learning.ResonanceManifold
	alphaCtrl     *AlphaController
	confidence    *errorCalibrator
	featureScale  map[string]*featureNormalizer
	alpha         float64
	featureSchema []string
	input         []float64
	pendingInput  []float64
	pendingMid    float64
	pendingAt     time.Time
	targetSamples uint64
	horizonReach  int
}

func newSymbolState(initialAlpha float64) *symbolState {
	return &symbolState{
		alpha:        initialAlpha,
		alphaCtrl:    NewAlphaController(initialAlpha, minAlpha, maxAlpha),
		confidence:   newErrorCalibrator(),
		featureScale: make(map[string]*featureNormalizer),
		horizonReach: 1,
	}
}
