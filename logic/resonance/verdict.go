package resonance

import "github.com/theapemachine/symm/types"

/*
learningVerdict reports whether the return head can publish a forecast. Evidence
strength belongs to Forecast.Confidence and never changes prediction availability.
*/
func learningVerdict(forecast *types.ResonanceForecast) (string, float64) {
	if forecast == nil {
		return "observing", 0
	}

	return "predicting", 1
}

/*
resonanceVerdict reduces one settled frame to the three readings the predictive
coding panel leads with.

Direction is the raw sign of the expected return and conviction is its current
posterior direction probability. Historical prequential skill remains a separate
reading. The tuning label names square-root recursive least squares with
covariance-derived gain.
*/
func resonanceVerdict(forecast *types.ResonanceForecast) types.ResonanceVerdict {
	learning, learningHealth := learningVerdict(forecast)
	verdict := types.ResonanceVerdict{
		Learning:       learning,
		Tuning:         "recursive least squares",
		LearningHealth: learningHealth,
		TuningHealth:   1,
	}

	if forecast == nil {
		return verdict
	}

	if forecast.ExpectedReturn > 0 {
		verdict.Direction = 1
	}

	if forecast.ExpectedReturn < 0 {
		verdict.Direction = -1
	}

	verdict.Conviction = forecast.Confidence

	return verdict
}
