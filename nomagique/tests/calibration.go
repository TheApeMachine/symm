package tests

import (
	"fmt"
	"github.com/theapemachine/symm/nomagique/core"
	"math"
	"testing"
)

func referenceCalibrationValidate(pair referenceCalibrationLearningPair, name string) (float64, float64, error) {
	if !referenceRLSFinite(pair.Predicted) || !referenceRLSFinite(pair.Actual) || pair.Predicted == 0 || pair.Actual == 0 {
		return 0, 0, fmt.Errorf("%s: invalid pair", name)
	}
	return pair.Predicted, pair.Actual, nil
}

// CheckCalibration compares full transitions to the unchanged source policies,
// including its intentionally non-cardinal initial range counter.
func CheckCalibration(t *testing.T, node core.Primitive, kind string) {
	t.Helper()
	trust := referenceCalibrationNewTrustWeight()
	ratio := referenceCalibrationSampleRatio()
	forecast := referenceCalibrationForecast()
	for i := 0; i < 100; i++ {
		predicted := 10.0 + float64(i%5)
		residual := 0.0
		if i > 7 {
			residual = 0.4 * math.Sin(float64(i)*0.17)
		}
		actual := predicted + residual
		pair := referenceCalibrationLearningPair{Predicted: predicted, Actual: actual}
		expected := map[string]float64{}
		var err error
		switch kind {
		case "trust":
			var out referenceCalibrationTrustWeightOutput
			out, err = trust.Measure(pair)
			expected = map[string]float64{"trust": out.Trust, "rate": out.Rate, "count": float64(out.Count), "value": out.Value}
		case "ratio":
			var out referenceCalibrationSampleRatioOutput
			out, err = ratio.Measure(pair)
			expected = map[string]float64{"value": out.Value, "peak_ratio": out.PeakRatio, "count": float64(out.Count)}
		case "forecast":
			var out referenceCalibrationForecastOutput
			out, err = forecast.Measure(pair)
			expected = map[string]float64{"value": out.Value, "scale": out.Scale, "trust": out.Trust, "rate": out.Rate, "count": float64(out.Count), "weight_count": float64(out.WeightCount)}
		default:
			t.Fatal("unknown reference")
		}
		if err != nil {
			t.Fatal(err)
		}
		out := Drain(t, node, Values(Record(map[string]any{"predicted": predicted, "actual": actual})))
		Sound(t, node)
		f := Fields(t, out[0])
		for name, want := range expected {
			EqualNumber(t, Number(t, f, name), want)
		}
	}
}
