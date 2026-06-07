package perspectives

import "github.com/theapemachine/symm/market/perspectives/types"

/*
Feedback is the top-down correction a signal receives where it measures: how
wrong the fused price predictions it contributed to have been (MSE, signed
Bias), and the Scale correction for the values it derives confidence and
surprise from. Signals consume this to retune their internal observation
scaling — feedback tunes inputs, it never limits outputs.
*/
type Feedback interface {
	MSE() float64
	Scale() float64
	Bias() float64
	Samples() int
}

/*
SourceFeedbackOf returns the live learned feedback view for one source — the
read side of the prediction loop, served through the contract every signal's
Measure method accepts.
*/
func SourceFeedbackOf(source types.SourceType) Feedback {
	return sourceFeedbackView{source: source}
}

type sourceFeedbackView struct {
	source types.SourceType
}

func (view sourceFeedbackView) MSE() float64 {
	return types.CurrentSourceFeedback(view.source).MSE
}

func (view sourceFeedbackView) Scale() float64 {
	return types.CurrentSourceFeedback(view.source).Scale
}

func (view sourceFeedbackView) Bias() float64 {
	return types.CurrentSourceFeedback(view.source).Bias
}

func (view sourceFeedbackView) Samples() int {
	return types.CurrentSourceFeedback(view.source).Samples
}
