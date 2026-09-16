package learning_test

import (
	"testing"

	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/learning"
)

func TestLearningPrimitiveIntegration(t *testing.T) {
	pair := learning.Pair{Predicted: 10, Actual: 10}

	trustEval := learning.NewTrustWeight()
	var trust learning.TrustReading

	for out := range trustEval.Next(sequence.NewValues(pair).Next(nil)) {
		trust = *(*learning.TrustReading)(out)
	}

	err := trustEval.Error()
	if err != nil || trust.Value != 1 {
		t.Fatalf("trust: %g, %v", trust.Value, err)
	}

	ratioEval := learning.NewSampleRatio()
	var ratio learning.RatioReading

	for out := range ratioEval.Next(sequence.NewValues(pair).Next(nil)) {
		ratio = *(*learning.RatioReading)(out)
	}

	err = ratioEval.Error()
	if err != nil || ratio.Value != 1 {
		t.Fatalf("ratio: %g, %v", ratio.Value, err)
	}

	forecastEval := learning.NewForecast()
	var forecast learning.ForecastReading

	for out := range forecastEval.Next(sequence.NewValues(pair).Next(nil)) {
		forecast = *(*learning.ForecastReading)(out)
	}

	err = forecastEval.Error()
	if err != nil || forecast.Value != 1 {
		t.Fatalf("forecast: %g, %v", forecast.Value, err)
	}
}
