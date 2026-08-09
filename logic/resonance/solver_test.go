package resonance

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/types"
)

const testAlpha = 0.05

func TestNewSolver(t *testing.T) {
	Convey("Given a configured resonance pace", t, func() {
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		Convey("It should retain the pace and create an empty symbol registry", func() {
			So(solver.alpha, ShouldEqual, testAlpha)
			_, found := solver.coders.Load("BTC/USD")
			So(found, ShouldBeFalse)
		})
	})
}

func TestUpdate(t *testing.T) {
	viper.Set("resonance.learning_rate", testAlpha)

	Convey("Given an invalid configured learning pace", t, func() {
		solver := NewSolver(t.Context(), nil, nil, 0)
		err := solver.Update(types.NewThesis(t.Context(), nil))

		Convey("It should fail before settling market state", func() {
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given finite normalized measurements for one symbol", t, func() {
		first := 0.25
		second := -0.5
		third := 0.1
		fourth := -0.2
		fifth := 0.3
		sixth := -0.4
		notFinite := math.Inf(1)
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"second":   {Normalized: &second},
				"first":    {Normalized: &first},
				"third":    {Normalized: &third},
				"fourth":   {Normalized: &fourth},
				"fifth":    {Normalized: &fifth},
				"sixth":    {Normalized: &sixth},
				"raw_only": {Raw: 100},
				"invalid":  {Normalized: &notFinite},
			},
		}})
		solver := NewSolver(t.Context(), make(chan []byte, 1), nil, testAlpha)

		err := solver.Update(thesis)

		Convey("It should settle and publish the hierarchy directly", func() {
			So(err, ShouldBeNil)
			So(thesis.Readiness.Resonance, ShouldBeTrue)

			stored, found := thesis.Resonance.Load("BTC/USD")
			So(found, ShouldBeTrue)

			reading, ok := stored.(types.ResonanceReading)
			So(ok, ShouldBeTrue)
			So(reading.Stage, ShouldEqual, "resonance")
			So(reading.Source, ShouldEqual, types.SourceResonance)
			So(reading.Symbol, ShouldEqual, "BTC/USD")
			So(reading.Alpha, ShouldEqual, testAlpha)
			So(reading.Layers, ShouldHaveLength, 3)
			So(reading.Layers[0].State, ShouldHaveLength, 6)
			So(reading.Layers[1].State, ShouldHaveLength, 12)
			So(reading.Layers[2].State, ShouldHaveLength, 6)
			So(reading.Latent, ShouldHaveLength, 6)
			So(math.IsNaN(reading.Surprise), ShouldBeFalse)
			So(math.IsNaN(reading.Energy), ShouldBeFalse)
		})

		Convey("It should retain the model while the schema is unchanged", func() {
			storedCoder, found := solver.coders.Load("BTC/USD")
			So(found, ShouldBeTrue)

			first = 0.75
			second = -0.25
			So(solver.Update(thesis), ShouldBeNil)

			updatedCoder, found := solver.coders.Load("BTC/USD")
			So(found, ShouldBeTrue)
			So(updatedCoder, ShouldEqual, storedCoder)
		})
	})

	Convey("Given normalized measurements for independent symbols", t, func() {
		bitcoin := 0.25
		ethereum := -0.25
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{
			{
				Source: types.SourceLiquidity,
				Symbol: "BTC/USD",
				Metrics: map[string]types.MetricSample{
					"first":  {Normalized: &bitcoin},
					"second": {Normalized: &bitcoin},
					"third":  {Normalized: &bitcoin},
					"fourth": {Normalized: &bitcoin},
					"fifth":  {Normalized: &bitcoin},
					"sixth":  {Normalized: &bitcoin},
				},
			},
			{
				Source: types.SourceLiquidity,
				Symbol: "ETH/USD",
				Metrics: map[string]types.MetricSample{
					"first":  {Normalized: &ethereum},
					"second": {Normalized: &ethereum},
					"third":  {Normalized: &ethereum},
					"fourth": {Normalized: &ethereum},
					"fifth":  {Normalized: &ethereum},
					"sixth":  {Normalized: &ethereum},
				},
			},
		})
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		err := solver.Update(thesis)

		Convey("It should retain one manifold per symbol", func() {
			So(err, ShouldBeNil)
			bitcoinCoder, bitcoinFound := solver.coders.Load("BTC/USD")
			ethereumCoder, ethereumFound := solver.coders.Load("ETH/USD")
			So(bitcoinFound, ShouldBeTrue)
			So(ethereumFound, ShouldBeTrue)
			So(bitcoinCoder, ShouldNotEqual, ethereumCoder)
			_, bitcoinPublished := thesis.Resonance.Load("BTC/USD")
			_, ethereumPublished := thesis.Resonance.Load("ETH/USD")
			So(bitcoinPublished, ShouldBeTrue)
			So(ethereumPublished, ShouldBeTrue)
		})
	})

	Convey("Given no normalized measurements", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"midpoint": {Raw: 100},
			},
		}})
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		err := solver.Update(thesis)

		Convey("It should leave resonance unstamped", func() {
			So(err, ShouldBeNil)
			So(thesis.Readiness.Resonance, ShouldBeFalse)
			_, published := thesis.Resonance.Load("BTC/USD")
			So(published, ShouldBeFalse)
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Given an active solver", t, func() {
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		Convey("It should cancel its context", func() {
			So(solver.Close(), ShouldBeNil)
			So(solver.ctx.Err(), ShouldEqual, context.Canceled)
		})
	})
}
