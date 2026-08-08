package resonance

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/vector"
	"gonum.org/v1/gonum/stat/distuv"
)

const chanceForecastSkill = 0.5

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
	skillEvidence float64
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
		alpha:         initialAlpha,
		skill:         probability.NewBernoulli(),
		skillEvidence: chanceForecastSkill,
		featureScale:  make(map[string]*adaptive.Standardizer),
		horizonReach:  1,
	}
}

/*
measureForecastSkill records whether a strictly prior return forecast beat the
zero-return baseline. A tie carries no evidence. Confidence is the Beta posterior
probability that the model's win rate exceeds chance, so sparse evidence stays
uncertain without a fixed warm-up count.
*/
func (state *symbolState) measureForecastSkill(predicted, actual float64) error {
	if math.IsNaN(predicted) || math.IsInf(predicted, 0) ||
		math.IsNaN(actual) || math.IsInf(actual, 0) {
		return fmt.Errorf("resonance: forecast skill requires finite returns")
	}

	modelError := actual - predicted
	modelLoss := modelError * modelError
	baselineLoss := actual * actual

	if modelLoss == baselineLoss {
		return nil
	}

	outcome := 0.0

	if modelLoss < baselineLoss {
		outcome = 1
	}

	if _, err := state.skill.Measure(outcome); err != nil {
		return fmt.Errorf("resonance: forecast skill observation: %w", err)
	}

	confidence, err := state.skill.ProbabilityAbove(chanceForecastSkill)

	if err != nil {
		return fmt.Errorf("resonance: forecast skill confidence: %w", err)
	}

	state.skillEvidence = confidence

	return nil
}

/*
forecastConfidence returns the posterior probability that the resolved return has
the sign of the current RLS point forecast. Unlike skillEvidence, this probability
belongs to this design point and widens for latent states with high coefficient
leverage. Before residual noise is identifiable, symmetry supplies the explicit
chance prior without pretending the uncertainty scale is known.
*/
func (state *symbolState) forecastConfidence(forecast learning.RLSOutput) (float64, error) {
	if math.IsNaN(forecast.Value) || math.IsInf(forecast.Value, 0) {
		return 0, fmt.Errorf("resonance: forecast value must be finite")
	}

	if !forecast.Ready {
		return chanceForecastSkill, nil
	}

	if !(forecast.Scale > 0) || math.IsNaN(forecast.Scale) ||
		math.IsInf(forecast.Scale, 0) || !(forecast.DegreesOfFreedom > 0) ||
		math.IsNaN(forecast.DegreesOfFreedom) ||
		math.IsInf(forecast.DegreesOfFreedom, 0) {
		return 0, fmt.Errorf("resonance: forecast distribution must be finite and positive")
	}

	if forecast.Value == 0 {
		return chanceForecastSkill, nil
	}

	distribution := distuv.StudentsT{
		Mu:    forecast.Value,
		Sigma: forecast.Scale,
		Nu:    forecast.DegreesOfFreedom,
	}

	if forecast.Value > 0 {
		return distribution.Survival(0), nil
	}

	return distribution.CDF(0), nil
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
