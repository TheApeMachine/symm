package learning_test

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
)

func TestLearningPrimitiveIntegration(t *testing.T) {
	pair := [2]float64{10, 10}

	trustNode := learning.NewTrustWeight()
	trust := data.Read[[4]float64](trustNode.Next(data.NewValue(pair).Next(nil)))

	if err := trustNode.Error(); err != nil || trust[0] != 1 {
		t.Fatalf("trust: %g, %v", trust[0], err)
	}

	ratioNode := learning.NewSampleRatio()
	ratio := data.Read[[3]float64](ratioNode.Next(data.NewValue(pair).Next(nil)))

	if err := ratioNode.Error(); err != nil || ratio[0] != 1 {
		t.Fatalf("ratio: %g, %v", ratio[0], err)
	}

	forecastNode := learning.NewForecast()
	forecast := data.Read[[6]float64](forecastNode.Next(data.NewValue(pair).Next(nil)))

	if err := forecastNode.Error(); err != nil || forecast[0] != 1 {
		t.Fatalf("forecast: %g, %v", forecast[0], err)
	}
}
