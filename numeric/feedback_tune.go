package numeric

import "fmt"

/*
FeedbackTuner applies top-down prediction feedback to a surprise threshold and
classifier weights exactly once per newer sample set for a matching symbol.
*/
type FeedbackTuner struct {
	lastSamples int
	mseGain     float64
}

func NewFeedbackTuner() *FeedbackTuner {
	return &FeedbackTuner{mseGain: 0.05}
}

func (tuner *FeedbackTuner) Apply(
	symbol string,
	feedbackSymbol string,
	samples int,
	mse float64,
	scale float64,
	bias float64,
	weights *ClassifierWeights,
) (applied bool, err error) {
	if samples <= 0 {
		return false, nil
	}

	if feedbackSymbol != symbol {
		return false, nil
	}

	if samples <= tuner.lastSamples {
		return false, nil
	}

	if weights == nil {
		return false, fmt.Errorf("numeric: FeedbackTuner requires weights")
	}

	tuner.lastSamples = samples

	adjustedThreshold := weights.Threshold

	if mse > 0 {
		adjustedThreshold += mse * tuner.mseGain
	}

	weights.Threshold = clamp(adjustedThreshold, 1.0, 5.0)

	learningRate := clamp(0.01*scale, 0.001, 0.1)
	adjustment := bias * learningRate

	weights.WIgnVol += adjustment
	weights.WIgnPrec += adjustment
	weights.WCoilComp += adjustment
	weights.WCoilPrec += adjustment
	weights.WOrgPrec += adjustment
	weights.WOrgComp += adjustment * 0.5
	weights.WOrgVol += adjustment * 0.5
	weights.WExVol -= adjustment
	weights.WExPrec -= adjustment
	weights.clamp()

	return true, nil
}

func (tuner *FeedbackTuner) LastSamples() int {
	return tuner.lastSamples
}

func (tuner *FeedbackTuner) Reset() error {
	tuner.lastSamples = 0

	return nil
}
