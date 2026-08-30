package statistic

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

const qualityPrefix = "quality/test"

func qualityMachine() types.Primitive {
	return types.Pipe(
		temporal.Window(qualityPrefix),
		ZScore(qualityPrefix),
		Baseline(qualityPrefix),
		QualityFrom(qualityPrefix),
	)
}

func qualityObservationForTest(value float64, sec float64) types.Frame {
	series := temporal.NewSeries(qualityPrefix)

	input := types.Frame{}
	input.Put(series.ValueSymbol, value)
	input.Put(series.SecSymbol, sec)
	input.Put(series.NsecSymbol, 0)

	return input
}

func TestQualityFrom(t *testing.T) {
	Convey("Given an estimator that has not yet built a baseline", t, func() {
		stream := types.NewStream(qualityMachine(), types.Frame{})

		Convey("It should leave both quality slots absent on the first observation", func() {
			output := stream.Step(qualityObservationForTest(100, 1000))
			So(output.Err, ShouldBeNil)

			_, hasDivergence := output.Get(SymbolDivergence)
			So(hasDivergence, ShouldBeFalse)

			_, hasNoise := output.Get(SymbolNoiseVariance)
			So(hasNoise, ShouldBeFalse)
		})
	})

	Convey("Given an estimator that has settled on a baseline", t, func() {
		stream := types.NewStream(qualityMachine(), types.Frame{})

		for sec := 1000; sec < 1010; sec++ {
			output := stream.Step(qualityObservationForTest(100, float64(sec)))
			So(output.Err, ShouldBeNil)
		}

		Convey("It should project the departure and its noise power when the value moves", func() {
			output := stream.Step(qualityObservationForTest(140, 1010))
			So(output.Err, ShouldBeNil)

			divergence, hasDivergence := output.Get(SymbolDivergence)
			So(hasDivergence, ShouldBeTrue)
			So(divergence, ShouldBeGreaterThan, 0)

			noise, hasNoise := output.Get(SymbolNoiseVariance)
			So(hasNoise, ShouldBeTrue)
			So(noise, ShouldBeGreaterThan, 0)
		})

		Convey("It should project the noise power as the dispersion squared", func() {
			output := stream.Step(qualityObservationForTest(140, 1010))
			So(output.Err, ShouldBeNil)

			dispersion, found := output.Get(
				types.MustIntern(temporal.JoinPrefix(qualityPrefix, "z/dispersion")),
			)
			So(found, ShouldBeTrue)

			noise, hasNoise := output.Get(SymbolNoiseVariance)
			So(hasNoise, ShouldBeTrue)
			So(noise, ShouldAlmostEqual, dispersion*dispersion, 1e-9)
		})

		Convey("It should carry the estimator's own residual as the divergence", func() {
			output := stream.Step(qualityObservationForTest(140, 1010))
			So(output.Err, ShouldBeNil)

			residual, found := output.Get(
				types.MustIntern(temporal.JoinPrefix(qualityPrefix, "z/residual")),
			)
			So(found, ShouldBeTrue)

			divergence, hasDivergence := output.Get(SymbolDivergence)
			So(hasDivergence, ShouldBeTrue)
			So(divergence, ShouldAlmostEqual, residual, 1e-9)
		})

		Convey("It should yield a scalar SNR that recovers the z-score squared", func() {
			output := stream.Step(qualityObservationForTest(140, 1010))
			So(output.Err, ShouldBeNil)

			zscore, found := output.Get(
				types.MustIntern(temporal.JoinPrefix(qualityPrefix, "z/value")),
			)
			So(found, ShouldBeTrue)

			divergence, _ := output.Get(SymbolDivergence)
			noise, _ := output.Get(SymbolNoiseVariance)

			So(divergence*divergence/noise, ShouldAlmostEqual, zscore*zscore, 1e-9)
		})
	})

	Convey("Given a settled estimator observing exactly its own baseline", t, func() {
		stream := types.NewStream(qualityMachine(), types.Frame{})

		for sec := 1000; sec < 1010; sec++ {
			output := stream.Step(qualityObservationForTest(100, float64(sec)))
			So(output.Err, ShouldBeNil)
		}

		Convey("It should report a genuine zero departure rather than an absent one", func() {
			output := stream.Step(qualityObservationForTest(100, 1010))
			So(output.Err, ShouldBeNil)

			divergence, hasDivergence := output.Get(SymbolDivergence)
			So(hasDivergence, ShouldBeTrue)
			So(math.Abs(divergence), ShouldBeLessThan, 1e-9)
		})
	})
}
