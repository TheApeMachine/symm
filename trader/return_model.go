package trader

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
ReturnModel tracks empirical forward returns from settled predictions.
*/
type ReturnModel struct {
	mu    sync.Mutex
	byKey map[string]*returnBucket
	alpha float64
}

type returnBucket struct {
	ewma adaptive.AlphaEMA
}

/*
NewReturnModel creates an empty EWMA return model.
*/
func NewReturnModel() *ReturnModel {
	alpha := config.System.TrailRiskEMAAlpha

	if alpha <= 0 {
		alpha = 0.2
	}

	return &ReturnModel{
		byKey: make(map[string]*returnBucket),
		alpha: alpha,
	}
}

/*
Apply records one settled forecast or calibration probe outcome.
*/
func (model *ReturnModel) Apply(feedback engine.PredictionFeedback) {
	if feedback.Unanchored || feedback.Source == "" {
		return
	}

	actual := feedback.ActualReturn

	if actual <= 0 {
		return
	}

	key := returnModelKey(feedback.Source, feedback.Regime)

	model.mu.Lock()
	defer model.mu.Unlock()

	bucket := model.byKey[key]

	if bucket == nil {
		bucket = &returnBucket{}
		model.byKey[key] = bucket
	}

	_ = bucket.ewma.Update(actual, model.alpha)
}

/*
Predict estimates gross return from historical forward returns.
*/
func (model *ReturnModel) Predict(
	source, regime string,
	confidence float64,
) (grossReturn float64, ok bool) {
	if confidence <= 0 {
		return 0, false
	}

	minSamples := config.System.MinCalibrationSamples

	if minSamples <= 0 {
		minSamples = 12
	}

	key := returnModelKey(source, regime)

	model.mu.Lock()
	bucket := model.byKey[key]
	model.mu.Unlock()

	if bucket == nil || bucket.ewma.Updates() < minSamples || bucket.ewma.Value() <= 0 {
		return 0, false
	}

	gross := confidence * bucket.ewma.Value()

	if gross <= 0 {
		return 0, false
	}

	return gross, true
}

func returnModelKey(source, regime string) string {
	if regime == "" {
		return source
	}

	return source + "|" + regime
}

func candidateUtility(candidate SignalCandidate) float64 {
	return candidate.Confidence * math.Max(candidate.ExpectedReturn, 1e-9)
}
