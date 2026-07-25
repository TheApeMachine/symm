package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
	pmanifold "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
TestProjectPrefersResonanceOverForecast ensures logic-layer resonance scalars
win when both forecast and resonance are present for the same symbol.
*/
func TestProjectPrefersResonanceOverForecast(t *testing.T) {
	Convey("Given forecast and resonance for one symbol", t, func() {
		thesis := types.NewThesis()
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
		mark, err := decimal.NewFromString("110.123456789123456789")
		So(err, ShouldBeNil)

		evidence := NewEvidence().Project(thesis, types.Holding{
			Symbol:     "AAA/USD",
			Mark:       mark,
			StopMark:   mark,
			EntryPrice: decimal.NewFromFloat64(100),
		})

		Convey("Then Present uses StopMark and prefers resonance", func() {
			So(evidence.Present, ShouldBeTrue)
			So(evidence.ReferencePrice.String(), ShouldEqual, mark.String())
			So(evidence.ExpectedReturn, ShouldEqual, 0.04)
			So(evidence.Uncertainty, ShouldEqual, 0.05)
		})
	})
}

/*
TestProjectRetreatPressureFromToxicity copies retreating_quantity onto Evidence
so Stoploss can gate quote-only marks.
*/
func TestProjectRetreatPressureFromToxicity(t *testing.T) {
	Convey("Given a toxicity retreat measurement", t, func() {
		thesis := types.NewThesis()
		pressure := 0.87
		thesis.Measurements = append(thesis.Measurements,
			&types.Measurement{
				Source: types.SourceToxicity,
				Symbol: "AAA/USD",
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricTouchQuantity, types.SideNone): {},
					types.MetricKey(types.MetricRetreatingQuantity, types.SideNone): {
						Normalized: &pressure,
						Raw:        870,
					},
				},
			},
		)

		evidence := NewEvidence().Project(thesis, types.Holding{
			Symbol:     "AAA/USD",
			Mark:       decimal.NewFromFloat64(100),
			StopMark:   decimal.NewFromFloat64(100),
			EntryPrice: decimal.NewFromFloat64(100),
		})

		Convey("Then retreat pressure is projected", func() {
			So(evidence.RetreatReady, ShouldBeTrue)
			So(evidence.RetreatPressure, ShouldEqual, 0.87)
		})
	})

	Convey("Given a current toxicity touch without any retreat", t, func() {
		thesis := types.NewThesis()
		thesis.Measurements = append(thesis.Measurements, &types.Measurement{
			Source: types.SourceToxicity,
			Symbol: "AAA/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricTouchQuantity, types.SideNone): {},
			},
		})

		evidence := NewEvidence().Project(thesis, types.Holding{
			Symbol:     "AAA/USD",
			StopMark:   decimal.NewFromFloat64(95),
			EntryPrice: decimal.NewFromFloat64(100),
		})

		Convey("Then the prior retreat gate can clear at this observed touch", func() {
			So(evidence.RetreatReady, ShouldBeTrue)
			So(evidence.RetreatPressure, ShouldEqual, 0)
		})
	})
}

/*
TestProjectForecastEpochFromSourceEpoch copies forecast provenance onto Evidence.
*/
func TestProjectForecastEpochFromSourceEpoch(t *testing.T) {
	Convey("Given a forecast with SourceEpoch", t, func() {
		thesis := types.NewThesis()
		thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
			Symbol:         "AAA/USD",
			SourceEpoch:    42,
			ExpectedReturn: 0.01,
			Uncertainty:    0.02,
			IncrementalMSE: 0.01,
			Ready:          true,
			Calibrated:     true,
		})

		evidence := NewEvidence().Project(thesis, types.Holding{
			Symbol:     "AAA/USD",
			Mark:       decimal.NewFromFloat64(110),
			StopMark:   decimal.NewFromFloat64(110),
			EntryPrice: decimal.NewFromFloat64(100),
		})

		Convey("Then epoch and normalized residual are projected", func() {
			So(evidence.ForecastEpoch, ShouldEqual, 42)
			So(evidence.NormalizedResidual, ShouldEqual, 5)
		})
	})
}

/*
TestProjectAbsentWithoutStopMark freezes Present when only bid Mark is set so
ask-entry vs bid cannot invent a stop breach.
*/
func TestProjectAbsentWithoutStopMark(t *testing.T) {
	Convey("Given inventory with bid Mark but no StopMark", t, func() {
		thesis := types.NewThesis()
		evidence := NewEvidence().Project(thesis, types.Holding{
			Symbol:     "AAA/USD",
			Mark:       decimal.NewFromFloat64(100),
			EntryPrice: decimal.NewFromFloat64(100),
			EntryAt:    ptrTime(time.Now()),
		})

		Convey("Then Present stays false", func() {
			So(evidence.Present, ShouldBeFalse)
		})
	})
}

/*
TestProjectAbsentWithoutMark freezes Present when inventory lacks a mark.
*/
func TestProjectAbsentWithoutMark(t *testing.T) {
	Convey("Given inventory without StopMark", t, func() {
		thesis := types.NewThesis()
		evidence := NewEvidence().Project(thesis, types.Holding{
			Symbol:     "AAA/USD",
			EntryPrice: decimal.NewFromFloat64(100),
			EntryAt:    ptrTime(time.Now()),
		})

		Convey("Then Present stays false", func() {
			So(evidence.Present, ShouldBeFalse)
		})
	})
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

/*
BenchmarkProject measures Evidence projection cost on the regulate hot path.
*/
func BenchmarkProject(b *testing.B) {
	thesis := types.NewThesis()
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
		StopMark:   decimal.NewFromFloat64(100),
		EntryPrice: decimal.NewFromFloat64(100),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = NewEvidence().Project(thesis, holding)
	}
}

func TestProjectManifoldSpreadStaysReturnSpace(t *testing.T) {
	Convey("Given a GasReady manifold state with return-space spread", t, func() {
		thesis := types.NewThesis()
		thesis.Manifold.Store("AAA/USD", manifold.State{
			Source:         "manifold",
			Symbol:         "AAA/USD",
			At:             time.Unix(1, 0).UTC(),
			Duration:       time.Second,
			Epoch:          1,
			ReferencePrice: decimal.NewFromInt64(100),
			Spread:         0.004,
			BuyCapacity:    decimal.NewFromInt64(50),
			SellCapacity:   decimal.NewFromInt64(50),
			InvalidReason:  manifold.Valid,
			BuyIntensity:   1,
			SellIntensity:  0.5,
			SpectralRadius: 0.1,
			Reading: pmanifold.Reading{
				PressureGradX: 0.1,
				Divergence:    -0.1,
				CoherenceMag2: 0.5,
				GuidanceSpeed: 0.1,
			},
		})

		evidence := NewEvidence().Project(thesis, types.Holding{
			Symbol:     "AAA/USD",
			Mark:       decimal.NewFromFloat64(100),
			StopMark:   decimal.NewFromFloat64(100),
			EntryPrice: decimal.NewFromFloat64(100),
		})

		Convey("Then spread stays in return space", func() {
			So(evidence.Spread, ShouldEqual, 0.004)
		})
	})
}
