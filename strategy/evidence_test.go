package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

/*
TestProjectPrefersResonanceOverForecast ensures logic-layer resonance scalars
win when both forecast and resonance are present for the same symbol.
*/
func TestProjectPrefersResonanceOverForecast(t *testing.T) {
	t.Parallel()

	thesis := types.NewThesis(nil, nil)
	thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
		Symbol:         "AAA/USD",
		ExpectedReturn: 0.01,
		Uncertainty:    0.02,
		IncrementalMSE: 0.03,
		Ready:          true,
		Calibrated:     true,
	})
	thesis.Resonance = append(thesis.Resonance, &logic.ResonanceOutcome{
		Symbol:         "AAA/USD",
		ExpectedReturn: 0.04,
		Uncertainty:    0.05,
		IncrementalMSE: 0.01,
		ReturnReady:    true,
	})

	evidence := Project(thesis, types.Holding{
		Symbol:     "AAA/USD",
		Mark:       decimal.NewFromFloat64(110),
		EntryPrice: decimal.NewFromFloat64(100),
	})

	if !evidence.Present {
		t.Fatal("expected present evidence")
	}

	if evidence.ExpectedReturn != 0.04 || evidence.Uncertainty != 0.05 {
		t.Fatalf("resonance not preferred: %+v", evidence)
	}
}

/*
TestProjectForecastEpochFromSourceEpoch copies forecast provenance onto Evidence.
*/
func TestProjectForecastEpochFromSourceEpoch(t *testing.T) {
	t.Parallel()

	thesis := types.NewThesis(nil, nil)
	thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
		Symbol:         "AAA/USD",
		SourceEpoch:    42,
		ExpectedReturn: 0.01,
		Uncertainty:    0.02,
		IncrementalMSE: 0.01,
		Ready:          true,
		Calibrated:     true,
	})

	evidence := Project(thesis, types.Holding{
		Symbol:     "AAA/USD",
		Mark:       decimal.NewFromFloat64(110),
		EntryPrice: decimal.NewFromFloat64(100),
	})

	if evidence.ForecastEpoch != 42 {
		t.Fatalf("forecast epoch: want 42, got %v", evidence.ForecastEpoch)
	}

	if evidence.NormalizedResidual != 0.5 {
		t.Fatalf("normalized residual: want 0.5, got %v", evidence.NormalizedResidual)
	}
}

/*
TestProjectAbsentWithoutMark freezes Present when inventory lacks a mark.
*/
func TestProjectAbsentWithoutMark(t *testing.T) {
	t.Parallel()

	thesis := types.NewThesis(nil, nil)
	evidence := Project(thesis, types.Holding{
		Symbol:     "AAA/USD",
		EntryPrice: decimal.NewFromFloat64(100),
		EntryAt:    ptrTime(time.Now()),
	})

	if evidence.Present {
		t.Fatal("mark-less holding must not be present")
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

/*
BenchmarkProject measures Evidence projection cost on the regulate hot path.
*/
func BenchmarkProject(b *testing.B) {
	thesis := types.NewThesis(nil, nil)
	thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
		Symbol:         "AAA/USD",
		ExpectedReturn: 0.01,
		Uncertainty:    0.02,
		IncrementalMSE: 0.01,
		Ready:          true,
		Calibrated:     true,
	})
	holding := types.Holding{
		Symbol:     "AAA/USD",
		Mark:       decimal.NewFromFloat64(100),
		EntryPrice: decimal.NewFromFloat64(100),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; b.Loop(); index++ {
		_ = Project(thesis, holding)
	}
}
