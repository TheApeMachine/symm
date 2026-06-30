package trader

import (
	"fmt"
	"math"
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

func TestRegimeSnapshotPayload(t *testing.T) {
	measurements := []*datura.Artifact{
		regimeMeasurement(logic.SourceFluid, map[logic.CategoryType]float64{
			logic.CategoryTurbulent: 0.6,
			logic.CategoryInertial:  0.2,
		}),
		regimeMeasurement(logic.SourceCorrelation, map[logic.CategoryType]float64{
			logic.CategoryDivergentStress: 0.4,
			logic.CategoryStochasticNoise: 0.7,
		}),
		regimeMeasurement(logic.SourcePumpDump, map[logic.CategoryType]float64{
			logic.CategoryOrganicTrend:      0.7,
			logic.CategoryCoiledCompression: 0.1,
		}),
		regimeMeasurement(logic.SourceCVD, map[logic.CategoryType]float64{
			logic.CategoryAggressiveDrive:   0.5,
			logic.CategoryStochasticBalance: 0.5,
		}),
		regimeMeasurement(logic.SourceHawkes, map[logic.CategoryType]float64{
			logic.CategorySaturation: 0.3,
		}),
		regimeMeasurement(logic.SourceSentiment, map[logic.CategoryType]float64{
			logic.CategoryRiskOnSurge:   0.3,
			logic.CategorySystemicSlump: 0.2,
		}),
		regimeMeasurement(logic.SourceToxicity, map[logic.CategoryType]float64{
			logic.CategoryHardSupport:     0.9,
			logic.CategoryLiquidityVacuum: 0.4,
			logic.CategoryToxicBluff:      0.3,
		}),
		regimeMeasurement(logic.SourceLiquidity, map[logic.CategoryType]float64{
			logic.CategoryRobustLiquidity: 0.6,
		}),
		regimeMeasurement(logic.SourceExhaustion, map[logic.CategoryType]float64{
			logic.CategoryThermalExhaustion: 0.9,
		}),
	}

	payload := regimeSnapshotPayload(42, measurements)
	if payload == nil {
		t.Fatal("regime payload was nil")
	}

	assertNear(t, payload["volatility"], 0.6)
	assertNear(t, payload["trend"], 0.4)
	assertNear(t, payload["bullish"], 0.6)
	assertNear(t, payload["bearish"], 0.6)
	assertNear(t, payload["choppiness"], 0.4)

	output, ok := payload["output"].(datura.Map[any])
	if !ok {
		t.Fatalf("output missing or wrong type: %#v", payload["output"])
	}

	assertNear(t, output["confidence"], 0.52)
	assertNear(t, output["strength"], 0.6)
	if output["status"] != "measured" {
		t.Fatalf("status = %v, want measured", output["status"])
	}
}

func TestRegimeSnapshotPayloadRequiresMeasurements(t *testing.T) {
	if payload := regimeSnapshotPayload(1, nil); payload != nil {
		t.Fatalf("empty measurements produced payload: %#v", payload)
	}
}

func regimeMeasurement(
	origin logic.SourceType,
	masses map[logic.CategoryType]float64,
) *datura.Artifact {
	measurement := datura.Acquire("test", datura.APPJSON).WithScope("BTC/USD")
	_ = measurement.SetOrigin(string(origin))

	for category, mass := range masses {
		measurement.MergeOutput(
			fmt.Sprintf("category.%d", logic.CategoryIndex(category)),
			mass,
		)
	}

	return measurement
}

func assertNear(t *testing.T, got any, want float64) {
	t.Helper()

	number, ok := got.(float64)
	if !ok {
		t.Fatalf("value = %#v, want float64 %.3f", got, want)
	}

	if math.Abs(number-want) > 1e-9 {
		t.Fatalf("value = %.12f, want %.12f", number, want)
	}
}
