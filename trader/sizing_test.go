package trader

import (
	"testing"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

func TestKellySizerSlotEUR(t *testing.T) {
	sizer := NewKellySizer(engine.DefaultCalibrationParams())

	for range config.System.MinCalibrationSamples {
		sizer.ApplyFeedback(engine.PredictionFeedback{
			Source:          "hawkes",
			Symbol:          "BTC/EUR",
			Regime:          "microstructure",
			PredictedReturn: 0.01,
			ActualReturn:    0.012,
		})
	}

	slot := sizer.SlotEUR(200, "hawkes", "microstructure", 0.8, 0.05)

	if slot <= config.System.MinCostEUR {
		t.Fatalf("expected positive Kelly slot, got %v", slot)
	}

	if slot > 200*config.System.MaxSlotPct/100+1e-9 {
		t.Fatalf("expected slot capped at MaxSlotPct, got %v", slot)
	}
}

/*
TestKellySizerColdSourceReturnsZero guards the no-cold-trading policy: a
(source, regime) slot with fewer settled samples than MinCalibrationSamples
must not produce a slot. Predictions are still recorded (spec step 4) and
feedback flows even without entries (step 6); trading only begins once the
calibrator has actually seen this combination settle.
*/
func TestKellySizerColdSourceReturnsZero(t *testing.T) {
	sizer := NewKellySizer(engine.DefaultCalibrationParams())
	slot := sizer.SlotEUR(200, "pumpdump", "microstructure", 1, 0)

	if slot != 0 {
		t.Fatalf("expected zero slot before MinCalibrationSamples settlements, got %v", slot)
	}
}

func TestKellySizerRejectsNegativeEdge(t *testing.T) {
	sizer := NewKellySizer(engine.DefaultCalibrationParams())

	for range config.System.MinCalibrationSamples {
		sizer.ApplyFeedback(engine.PredictionFeedback{
			Source:          "hawkes",
			Symbol:          "BTC/EUR",
			Regime:          "microstructure",
			PredictedReturn: 0.01,
			ActualReturn:    -0.02,
		})
	}

	slot := sizer.SlotEUR(200, "hawkes", "microstructure", 0.8, 0.05)

	if slot != 0 {
		t.Fatalf("expected zero slot after losing calibration, got %v", slot)
	}
}

func TestKellySizerTreatsFeeFloorAsLoss(t *testing.T) {
	sizer := NewKellySizer(engine.DefaultCalibrationParams())

	for range config.System.MinCalibrationSamples {
		sizer.ApplyFeedback(engine.PredictionFeedback{
			Source:          "hawkes",
			Symbol:          "BTC/EUR",
			Regime:          "microstructure",
			PredictedReturn: 0.01,
			ActualReturn:    roundTripFeeReturn() * 0.5,
		})
	}

	slot := sizer.SlotEUR(200, "hawkes", "microstructure", 0.8, 0.05)

	if slot != 0 {
		t.Fatalf("expected sub-fee positive returns to size as losses, got %v", slot)
	}
}

func TestKellySizerBranchesByRegime(t *testing.T) {
	sizer := NewKellySizer(engine.DefaultCalibrationParams())

	for range config.System.MinCalibrationSamples {
		sizer.ApplyFeedback(engine.PredictionFeedback{
			Source:          "hawkes",
			Symbol:          "BTC/EUR",
			Regime:          "trend",
			PredictedReturn: 0.01,
			ActualReturn:    0.012,
		})
		sizer.ApplyFeedback(engine.PredictionFeedback{
			Source:          "hawkes",
			Symbol:          "BTC/EUR",
			Regime:          "chop",
			PredictedReturn: 0.01,
			ActualReturn:    -0.02,
		})
	}

	trendSlot := sizer.SlotEUR(200, "hawkes", "trend", 0.8, 0.05)
	chopSlot := sizer.SlotEUR(200, "hawkes", "chop", 0.8, 0.05)

	if trendSlot <= config.System.MinCostEUR {
		t.Fatalf("expected trend slot, got %v", trendSlot)
	}

	if chopSlot != 0 {
		t.Fatalf("expected chop regime to reject sizing, got %v", chopSlot)
	}
}

func BenchmarkKellySizerSlotEUR(b *testing.B) {
	sizer := NewKellySizer(engine.DefaultCalibrationParams())

	for index := range config.System.MinCalibrationSamples {
		sizer.ApplyFeedback(engine.PredictionFeedback{
			Source:          "hawkes",
			Symbol:          "BTC/EUR",
			Regime:          "microstructure",
			PredictedReturn: 0.01,
			ActualReturn:    float64(index%3)*0.005 - 0.002,
		})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = sizer.SlotEUR(200, "hawkes", "microstructure", 0.8, 0.05)
	}
}

func TestTrustScale(t *testing.T) {
	if trustScale(0) != 1 {
		t.Fatal("expected unit trust on zero error")
	}

	if trustScale(1) >= trustScale(0.1) {
		t.Fatal("expected trust to fall as error rises")
	}
}
