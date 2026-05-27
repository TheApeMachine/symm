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
			PredictedReturn: 0.01,
			ActualReturn:    0.012,
		})
	}

	slot := sizer.SlotEUR(200, "hawkes", 0.8, 0.05)

	if slot <= config.System.MinCostEUR {
		t.Fatalf("expected positive Kelly slot, got %v", slot)
	}

	if slot > 200*config.System.MaxSlotPct/100+1e-9 {
		t.Fatalf("expected slot capped at MaxSlotPct, got %v", slot)
	}
}

func TestKellySizerColdSourceUsesMaxFraction(t *testing.T) {
	sizer := NewKellySizer(engine.DefaultCalibrationParams())
	slot := sizer.SlotEUR(200, "pumpdump", 1, 0)

	maxSlot := 200 * config.System.MaxSlotPct / 100

	if slot <= 0 || slot > maxSlot+1e-9 {
		t.Fatalf("expected cold-source slot within max fraction, got %v", slot)
	}
}

func TestKellySizerRejectsNegativeEdge(t *testing.T) {
	sizer := NewKellySizer(engine.DefaultCalibrationParams())

	for range config.System.MinCalibrationSamples {
		sizer.ApplyFeedback(engine.PredictionFeedback{
			Source:          "hawkes",
			Symbol:          "BTC/EUR",
			PredictedReturn: 0.01,
			ActualReturn:    -0.02,
		})
	}

	slot := sizer.SlotEUR(200, "hawkes", 0.8, 0.05)

	if slot != 0 {
		t.Fatalf("expected zero slot after losing calibration, got %v", slot)
	}
}

func BenchmarkKellySizerSlotEUR(b *testing.B) {
	sizer := NewKellySizer(engine.DefaultCalibrationParams())

	for index := range config.System.MinCalibrationSamples {
		sizer.ApplyFeedback(engine.PredictionFeedback{
			Source:          "hawkes",
			Symbol:          "BTC/EUR",
			PredictedReturn: 0.01,
			ActualReturn:    float64(index%3)*0.005 - 0.002,
		})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = sizer.SlotEUR(200, "hawkes", 0.8, 0.05)
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
