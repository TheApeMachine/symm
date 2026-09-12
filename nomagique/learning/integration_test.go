package learning_test

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestLearningPrimitiveIntegration(t *testing.T) {
	pair := learning.Pair{Predicted: 10, Actual: 10}

	trustEval := transport.NewEvaluate(learning.NewTrustWeight())
	var trust learning.TrustReading

	for out := range trustEval.Next(transport.NewValues(pair).Next(nil)) {
		trust = *(*learning.TrustReading)(out)
	}

	err := trustEval.Error()
	if err != nil || trust.Value != 1 {
		t.Fatalf("trust: %g, %v", trust.Value, err)
	}

	ratioEval := transport.NewEvaluate(learning.NewSampleRatio())
	var ratio learning.RatioReading

	for out := range ratioEval.Next(transport.NewValues(pair).Next(nil)) {
		ratio = *(*learning.RatioReading)(out)
	}

	err = ratioEval.Error()
	if err != nil || ratio.Value != 1 {
		t.Fatalf("ratio: %g, %v", ratio.Value, err)
	}

	forecastEval := transport.NewEvaluate(learning.NewForecast())
	var forecast learning.ForecastReading

	for out := range forecastEval.Next(transport.NewValues(pair).Next(nil)) {
		forecast = *(*learning.ForecastReading)(out)
	}

	err = forecastEval.Error()
	if err != nil || forecast.Value != 1 {
		t.Fatalf("forecast: %g, %v", forecast.Value, err)
	}
}
