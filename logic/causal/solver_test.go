package causal

import (
	"math"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestBuildCausalRow(t *testing.T) {
	convey.Convey("Given a causal thesis with non-finite measurement metrics", t, func() {
		solver := NewSolver(nil, nil)
		thesis := types.NewThesis()
		thesis.Resonance.Store("BTC/USD", map[string]any{
			"energy":       1.5,
			"surprise":     0.25,
			"forwardCurve": []float64{0.75},
		})
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
		thesis := types.NewThesis()
		thesis.Resonance.Store("BTC/USD", map[string]any{
			"energy":       1.5,
			"surprise":     0.25,
			"forwardCurve": []float64{0.75},
		})
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
