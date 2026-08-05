package causal

import (
	"math"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func testResonanceReading(
	t *testing.T,
	energy, surprise float64,
	curve []float64,
) types.ResonanceReading {
	retention := make([]float64, len(curve))

	for index := range retention {
		retention[index] = 1
	}

	forecast, err := types.NewResonanceForecast(
		curve, retention, len(curve), 0.75,
	)

	if err != nil {
		panic(err)
	}

	return types.ResonanceReading{
		Energy: energy, Surprise: surprise, Forecast: forecast,
		ForecastValidity: types.MeasurementValidity{
			State: types.ValidityValid, Readiness: types.ReadinessForecast,
		},
	}
}

func TestBuildCausalRow(t *testing.T) {
	convey.Convey("Given a causal thesis with non-finite measurement metrics", t, func() {
		solver := NewSolver(nil, nil)
		thesis := types.NewThesis(nil)
		thesis.Resonance.Store("BTC/USD", testResonanceReading(t,
			1.5, 0.25, []float64{0.75},
		))
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
			Metrics: map[string]types.MetricSample{
				"finite": {Raw: 2.5},
				"nan":    {Raw: math.NaN()},
			},
		}})

		convey.Convey("It should average only finite target evidence", func() {
			row, intervention, contagion, condition, ok := solver.buildCausalRow(thesis, "BTC/USD")

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(row, convey.ShouldResemble, []float64{1.5, 0.25, 0.75, 2.5})
			convey.So(intervention, convey.ShouldEqual, 0.75)
			convey.So(contagion, convey.ShouldEqual, 0.25)
			convey.So(condition, convey.ShouldEqual, 1.5)
		})
	})

	convey.Convey("Given a causal thesis with only non-finite target evidence", t, func() {
		solver := NewSolver(nil, nil)
		thesis := types.NewThesis(nil)
		thesis.Resonance.Store("BTC/USD", testResonanceReading(t,
			1.5, 0.25, []float64{0.75},
		))
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
			Metrics: map[string]types.MetricSample{
				"infinite": {Raw: math.Inf(1)},
				"nan":      {Raw: math.NaN()},
			},
		}})

		convey.Convey("It should skip the row instead of passing non-finite values to Pearl", func() {
			row, intervention, contagion, condition, ok := solver.buildCausalRow(thesis, "BTC/USD")

			convey.So(ok, convey.ShouldBeFalse)
			convey.So(row, convey.ShouldBeNil)
			convey.So(intervention, convey.ShouldEqual, 0)
			convey.So(contagion, convey.ShouldEqual, 0)
			convey.So(condition, convey.ShouldEqual, 0)
		})
	})
}

func TestBuildCausalInputs(t *testing.T) {
	convey.Convey("Given unsorted measurements in slice and single-row storage", t, func() {
		solver := NewSolver(nil, nil)
		thesis := types.NewThesis(nil)
		thesis.Resonance.Store("ZEC/USD", testResonanceReading(t,
			2.0, 0.2, []float64{0.02},
		))
		thesis.Resonance.Store("ADA/USD", testResonanceReading(t,
			1.0, 0.1, []float64{0.01},
		))
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "ZEC/USD",
			At:     time.Unix(1, 0),
			Metrics: map[string]types.MetricSample{
				"first":  {Raw: 2},
				"second": {Raw: 4},
			},
		}})
		thesis.Measurements.Store(types.SourceCVD, &types.Measurement{
			Source: types.SourceCVD,
			Symbol: "ADA/USD",
			At:     time.Unix(1, 0),
			Metrics: map[string]types.MetricSample{
				"single": {Raw: 8},
			},
		})

		convey.Convey("It should isolate target means and order symbols deterministically", func() {
			inputs := solver.buildCausalInputs(thesis)

			convey.So(len(inputs), convey.ShouldEqual, 2)
			convey.So(inputs[0].symbol, convey.ShouldEqual, "ADA/USD")
			convey.So(inputs[0].row, convey.ShouldResemble, []float64{1, 0.1, 0.01, 8})
			convey.So(inputs[1].symbol, convey.ShouldEqual, "ZEC/USD")
			convey.So(inputs[1].row, convey.ShouldResemble, []float64{2, 0.2, 0.02, 3})
		})
	})
}
