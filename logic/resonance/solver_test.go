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
		solver := NewSolver(t.Context(), nil, nil, -1.0)
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
		seventh := 0.6
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Readiness = types.Readiness{
			Correlation: true, CVD: true, DepthFlow: true, Exhaustion: true,
			Hawkes: true, LeadLag: true, Liquidity: true, PumpDump: true,
			Sentiment: true, Toxicity: true,
		}
		symbol.Measurements = []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"second":   {Normalized: &second},
				"first":    {Normalized: &first},
				"third":    {Normalized: &third},
				"fourth":   {Normalized: &fourth},
				"fifth":    {Normalized: &fifth},
				"sixth":    {Normalized: &sixth},
				"seventh":  {Normalized: &seventh},
				"raw_only": {Raw: 100},
			},
		}}
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(t.Context(), make(chan []byte, 1), nil, testAlpha)

		err := solver.Update(thesis)

		Convey("It should settle and publish the hierarchy directly", func() {
			So(err, ShouldBeNil)
			So(thesis.Stamped("BTC/USD", types.SourceResonance), ShouldBeTrue)

			stored, found := symbol.Resonance.Load("BTC/USD")
			So(found, ShouldBeTrue)

			reading, ok := stored.(types.ResonanceReading)
			So(ok, ShouldBeTrue)
			So(reading.Stage, ShouldEqual, "resonance")
			So(reading.Source, ShouldEqual, types.SourceResonance)
			So(reading.Symbol, ShouldEqual, "BTC/USD")
			So(reading.Alpha, ShouldEqual, testAlpha)
			So(reading.Layers, ShouldHaveLength, 3)
			So(reading.Layers[0].State, ShouldHaveLength, 7)
			So(reading.Layers[1].State, ShouldHaveLength, 14)
			So(reading.Layers[2].State, ShouldHaveLength, 7)
			So(reading.Latent, ShouldHaveLength, 7)
			So(reading.Embedding, ShouldHaveLength, 2)
			So(reading.Samples, ShouldEqual, 1)
			So(reading.Verdict.Learning, ShouldEqual, "observing")
			So(reading.Forecast, ShouldBeNil)
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
		for symbol, value := range map[string]float64{
			"BTC/USD": bitcoin,
			"ETH/USD": ethereum,
		} {
			value := value
			symbolState := types.NewSymbol(symbol, nil)
			symbolState.Readiness = types.Readiness{
				Correlation: true, CVD: true, DepthFlow: true, Exhaustion: true,
				Hawkes: true, LeadLag: true, Liquidity: true, PumpDump: true,
				Sentiment: true, Toxicity: true,
			}
			symbolState.Measurements = []*types.Measurement{{
				Source: types.SourceLiquidity,
				Symbol: symbol,
				Metrics: map[string]types.MetricSample{
					"first":  {Normalized: &value},
					"second": {Normalized: &value},
					"third":  {Normalized: &value},
					"fourth": {Normalized: &value},
					"fifth":  {Normalized: &value},
					"sixth":  {Normalized: &value},
				},
			}}
			thesis.Symbols.Store(symbol, symbolState)
		}
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		err := solver.Update(thesis)

		Convey("It should retain one manifold per symbol", func() {
			So(err, ShouldBeNil)
			bitcoinCoder, bitcoinFound := solver.coders.Load("BTC/USD")
			ethereumCoder, ethereumFound := solver.coders.Load("ETH/USD")
			So(bitcoinFound, ShouldBeTrue)
			So(ethereumFound, ShouldBeTrue)
			So(bitcoinCoder, ShouldNotEqual, ethereumCoder)
			bitcoinState, _ := thesis.Symbols.Load("BTC/USD")
			ethereumState, _ := thesis.Symbols.Load("ETH/USD")
			_, bitcoinPublished := bitcoinState.(*types.Symbol).Resonance.Load("BTC/USD")
			_, ethereumPublished := ethereumState.(*types.Symbol).Resonance.Load("ETH/USD")
			So(bitcoinPublished, ShouldBeTrue)
			So(ethereumPublished, ShouldBeTrue)
			So(thesis.Stamped("BTC/USD", types.SourceResonance), ShouldBeTrue)
			So(thesis.Stamped("ETH/USD", types.SourceResonance), ShouldBeTrue)
		})
	})

	Convey("Given a symbol populated through ready signal publications", t, func() {
		value := 0.25
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AddMeasurement(&types.Measurement{
			ID: "liquidity", Source: types.SourceLiquidity, Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"score": {Normalized: &value},
			},
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		sources := []types.SourceType{
			types.SourceCorrelation,
			types.SourceCVD,
			types.SourceDepthFlow,
			types.SourceExhaustion,
			types.SourceHawkes,
			types.SourceLeadLag,
			types.SourceLiquidity,
			types.SourcePumpDump,
			types.SourceSentiment,
			types.SourceToxicity,
		}

		for _, source := range sources {
			err := thesis.AppendMeasurements(source, []*types.Measurement{{
				ID: string(source), Source: source, Symbol: "BTC/USD",
				Metrics: map[string]types.MetricSample{
					"score": {Normalized: &value},
				},
			}}, true)
			So(err, ShouldBeNil)
		}

		solver := NewSolver(t.Context(), nil, nil, testAlpha)
		err := solver.Update(thesis)

		Convey("It should settle the published symbol instead of rejecting its readiness", func() {
			So(err, ShouldBeNil)
			_, found := symbol.Resonance.Load("BTC/USD")
			So(found, ShouldBeTrue)
		})
	})

	Convey("Given no normalized measurements", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Readiness = types.Readiness{
			Correlation: true, CVD: true, DepthFlow: true, Exhaustion: true,
			Hawkes: true, LeadLag: true, Liquidity: true, PumpDump: true,
			Sentiment: true, Toxicity: true,
		}
		symbol.Measurements = []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"midpoint": {Raw: 100},
			},
		}}
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		err := solver.Update(thesis)

		Convey("It should stamp the empty normalized feature set", func() {
			So(err, ShouldBeNil)
			So(thesis.Stamped("BTC/USD", types.SourceResonance), ShouldBeTrue)
			_, published := symbol.Resonance.Load("BTC/USD")
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

func BenchmarkUpdate(b *testing.B) {
	first := 0.25
	second := -0.5
	third := 0.1
	fourth := -0.2
	fifth := 0.3
	sixth := -0.4
	seventh := 0.6

	ctx := b.Context()
	thesis := types.NewThesis(ctx, nil)
	symbol := types.NewSymbol("BTC/USD", nil)
	symbol.Readiness = types.Readiness{
		Correlation: true, CVD: true, DepthFlow: true, Exhaustion: true,
		Hawkes: true, LeadLag: true, Liquidity: true, PumpDump: true,
		Sentiment: true, Toxicity: true,
	}
	symbol.Measurements = []*types.Measurement{{
		Source: types.SourceLiquidity,
		Symbol: "BTC/USD",
		Metrics: map[string]types.MetricSample{
			"second":  {Normalized: &second},
			"first":   {Normalized: &first},
			"third":   {Normalized: &third},
			"fourth":  {Normalized: &fourth},
			"fifth":   {Normalized: &fifth},
			"sixth":   {Normalized: &sixth},
			"seventh": {Normalized: &seventh},
		},
	}}
	thesis.Symbols.Store("BTC/USD", symbol)
	solver := NewSolver(ctx, nil, nil, testAlpha)
	sources := []types.SourceType{
		types.SourceCorrelation,
		types.SourceCVD,
		types.SourceDepthFlow,
		types.SourceExhaustion,
		types.SourceHawkes,
		types.SourceLeadLag,
		types.SourceLiquidity,
		types.SourcePumpDump,
		types.SourceSentiment,
		types.SourceToxicity,
	}
	b.ReportAllocs()

	for b.Loop() {
		symbol.Reset()

		for _, source := range sources {
			symbol.Stamp(source)
		}

		_ = solver.Update(thesis)
	}
}
