package engine

/*
ForwardSourceFeedback applies feedback when the source label matches.
*/
func ForwardSourceFeedback(
	source string,
	feedback PredictionFeedback,
	apply func(PredictionFeedback),
) {
	if feedback.Source != source {
		return
	}

	apply(feedback)
}

/*
ValidPredictionFeedback reports whether feedback can update calibration state.
*/
func ValidPredictionFeedback(feedback PredictionFeedback) bool {
	return feedback.Symbol != "" && feedback.PredictedReturn > 0
}
