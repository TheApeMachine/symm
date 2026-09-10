package learning_test

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestLearningPrimitiveIntegration(t *testing.T) {
	pair := learning.Pair{Predicted: 10, Actual: 10}

	trust, err := transport.Evaluate(learning.NewTrustWeight(), transport.Values(pair))
	if err != nil || trust.Value != 1 {
		t.Fatalf("trust: %g, %v", trust.Value, err)
	}

	ratio, err := transport.Evaluate(learning.NewSampleRatio(), transport.Values(pair))
	if err != nil || ratio.Value != 1 {
		t.Fatalf("ratio: %g, %v", ratio.Value, err)
	}

	forecast, err := transport.Evaluate(learning.NewForecast(), transport.Values(pair))
	if err != nil || forecast.Value != 1 {
		t.Fatalf("forecast: %g, %v", forecast.Value, err)
	}
}
