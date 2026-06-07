package types

import (
	"fmt"
	"math"
	"sync"

	"github.com/theapemachine/errnie"
)

/*
SourceFeedback reports how well one source's recent forecasts matched later
price movement. Scale is a bidirectional calibration multiplier learned from
settled predicted-vs-realized movement, so top-down feedback can sharpen timid
signals and soften overconfident ones.
*/
type SourceFeedback struct {
	MSE     float64
	Scale   float64
	Bias    float64 // signed mean settle error while this source contributed
	Samples int
}

var sourceFeedback sync.Map

/*
ResetSourceFeedback clears all learned source feedback. Tests and fresh replay
runs use this to avoid carrying live calibration between independent runs.
*/
func ResetSourceFeedback() {
	sourceFeedback.Clear()
}

/*
UpdateSourceFeedback stores the current error state for one source.
*/
func UpdateSourceFeedback(
	source SourceType,
	mse float64,
	scale float64,
	bias float64,
	samples int,
) (SourceFeedback, error) {
	if source == SourceNone {
		return SourceFeedback{}, errnie.Error(fmt.Errorf("perspectives: feedback source is empty"))
	}

	if invalidFeedbackValue(mse) || invalidFeedbackScale(scale) {
		return SourceFeedback{}, errnie.Error(fmt.Errorf(
			"perspectives: invalid feedback mse=%v scale=%v",
			mse,
			scale,
		))
	}

	if math.IsNaN(bias) || math.IsInf(bias, 0) {
		return SourceFeedback{}, errnie.Error(fmt.Errorf(
			"perspectives: invalid feedback bias=%v", bias,
		))
	}

	feedback := SourceFeedback{
		MSE:     mse,
		Scale:   scale,
		Bias:    bias,
		Samples: samples,
	}

	sourceFeedback.Store(source, feedback)

	return feedback, nil
}

/*
CurrentSourceFeedback returns the learned feedback for source. Before a source has
settled forecasts, scale is exactly one so live measurements are unchanged.
*/
func CurrentSourceFeedback(source SourceType) SourceFeedback {
	value, ok := sourceFeedback.Load(source)

	if !ok {
		return SourceFeedback{Scale: 1}
	}

	feedback, ok := value.(SourceFeedback)

	if !ok || feedback.Scale < 0 {
		return SourceFeedback{Scale: 1}
	}

	return feedback
}

/*
AdjustSourceValue applies learned top-down feedback to a source feature before
the signal derives category evidence and confidence from that feature.
*/
func AdjustSourceValue(source SourceType, value float64) float64 {
	if source == SourceNone || source == SourcePrediction || value == 0 {
		return value
	}

	feedback := CurrentSourceFeedback(source)

	if feedback.Samples <= 0 || feedback.Scale == 1 {
		return value
	}

	scale := feedback.Scale

	// Bounded: feedback tunes a signal's observation scale, it must never be
	// able to zero a signal out or blow it up.
	if scale < 0.5 {
		scale = 0.5
	}

	if scale > 2 {
		scale = 2
	}

	return value * scale
}

func invalidFeedbackValue(value float64) bool {
	return math.IsNaN(value) || math.IsInf(value, 0) || value < 0
}

func invalidFeedbackScale(value float64) bool {
	return math.IsNaN(value) || math.IsInf(value, 0) || value < 0
}
