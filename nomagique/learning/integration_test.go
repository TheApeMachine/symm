package learning_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestLearningPrimitiveIntegration(t *testing.T) {
	for name, graph := range map[string]core.Primitive{
		"trust": learning.NewTrustWeight(), "ratio": learning.NewSampleRatio(), "forecast": learning.NewForecast(),
	} {
		fields, err := transport.Evaluate[map[string]core.Primitive](graph, core.Record(map[string]any{"predicted": 10.0, "actual": 10.0}))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		value, err := core.Field[float64](fields, "value")
		if err != nil || value != 1 {
			t.Fatalf("%s: %g, %v", name, value, err)
		}
	}
}
