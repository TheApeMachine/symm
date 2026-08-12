package resonance

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/learning"
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
		symbol.AppendMeasurement(types.SourceLiquidity, &types.Measurement{
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
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(t.Context(), make(chan []byte, 1), nil, testAlpha)

		err := solver.Update(thesis)

		Convey("It should settle and publish the hierarchy directly", func() {
			So(err, ShouldBeNil)

			stored, found := symbol.Resonance.Load("BTC/USD")
			So(found, ShouldBeTrue)

			coder, ok := stored.(*learning.ResonanceManifold)
			So(ok, ShouldBeTrue)
			layers, surprise, energy := coder.WireSnapshot()
			So(layers, ShouldHaveLength, 3)
			So(layers[0].State, ShouldHaveLength, 7)
			So(layers[1].State, ShouldHaveLength, 14)
			So(layers[2].State, ShouldHaveLength, 7)
			So(math.IsNaN(surprise), ShouldBeFalse)
			So(math.IsNaN(energy), ShouldBeFalse)
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
			symbolState.AppendMeasurement(types.SourceLiquidity, &types.Measurement{
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
			})
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
		})
	})

	Convey("Given a symbol populated through ready signal publications", t, func() {
		value := 0.25
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AppendMeasurement(types.SourceLiquidity, &types.Measurement{
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
			symbol.AppendMeasurement(source, &types.Measurement{
				ID: string(source), Source: source, Symbol: "BTC/USD",
				Metrics: map[string]types.MetricSample{
					"score": {Normalized: &value},
				},
			})
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
		symbol.AppendMeasurement(types.SourceLiquidity, &types.Measurement{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"midpoint": {Raw: 100},
			},
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		err := solver.Update(thesis)

		Convey("It should publish the explicit observing state", func() {
			So(err, ShouldBeNil)
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
	symbol.AppendMeasurement(types.SourceLiquidity, &types.Measurement{
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
	})
	thesis.Symbols.Store("BTC/USD", symbol)
	solver := NewSolver(ctx, nil, nil, testAlpha)
	b.ReportAllocs()

	for b.Loop() {
		_ = solver.Update(thesis)
	}
}
