package resonance

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/vector"
)

/*
symbolState holds one symbol's predictive-coding model, feature normalization,
prequential skill evidence, and unresolved return target.
*/
type symbolState struct {
	manifold      *learning.ResonanceManifold
	skill         *probability.Bernoulli
	featureScale  map[string]*adaptive.Standardizer
	extractor     *vector.FeatureExtractor
	alpha         float64
	confidence    float64
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
		alpha:        initialAlpha,
		skill:        probability.NewBernoulli(),
		featureScale: make(map[string]*adaptive.Standardizer),
		horizonReach: 1,
	}
}

/*
measureForecastSkill records whether a strictly prior return forecast beat the
zero-return baseline. A tie contributes neutral evidence. Confidence is the Beta
posterior probability that the model's win rate exceeds chance, so sparse evidence
stays uncertain without a fixed warm-up count.
*/
func (state *symbolState) measureForecastSkill(predicted, actual float64) error {
	if math.IsNaN(predicted) || math.IsInf(predicted, 0) ||
		math.IsNaN(actual) || math.IsInf(actual, 0) {
		return fmt.Errorf("resonance: forecast skill requires finite returns")
	}

	modelError := actual - predicted
	modelLoss := modelError * modelError
	baselineLoss := actual * actual
	outcome := 0.5

	if modelLoss < baselineLoss {
		outcome = 1
	}

	if modelLoss > baselineLoss {
		outcome = 0
	}

	if _, err := state.skill.Measure(outcome); err != nil {
		return fmt.Errorf("resonance: forecast skill observation: %w", err)
	}

	confidence, err := state.skill.ProbabilityAbove(0.5)

	if err != nil {
		return fmt.Errorf("resonance: forecast skill confidence: %w", err)
	}

	state.confidence = confidence

	return nil
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
