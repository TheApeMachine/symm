package resonance

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

const testAlpha = 0.05

func appendNormalizedMeasurement(
	symbol *types.Symbol, name string,
	source types.SourceType,
	values map[string]*float64,
) {
	metrics := make(map[string]types.MetricSample, len(values))

	for key, value := range values {
		metrics[key] = types.MetricSample{Normalized: value}
	}

	symbol.AppendMeasurement(source, &types.Measurement{
		Source:  source,
		Symbol:  name,
		Metrics: metrics,
	})
}

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

	Convey("Given a varied normalized measurement sequence for one symbol", t, func() {
		first := 0.25
		second := -0.5
		third := 0.1
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(t.Context(), make(chan []byte, 1), nil, testAlpha)
		appendMeasurement := func() {
			appendNormalizedMeasurement(symbol, "BTC/USD", types.SourceLiquidity, map[string]*float64{
				"second": &second,
				"first":  &first,
				"third":  &third,
			})
		}

		appendMeasurement()
		So(solver.Update(thesis), ShouldBeNil)
		first = 0.75
		second = -0.25
		appendMeasurement()
		So(solver.Update(thesis), ShouldBeNil)
		first = 0.5
		second = -0.75
		appendMeasurement()

		err := solver.Update(thesis)

		Convey("It should settle and publish once prior feature spread exists", func() {
			So(err, ShouldBeNil)

			stored, found := symbol.Resonance.Load("BTC/USD")
			So(found, ShouldBeTrue)

			coder, ok := stored.(*learning.ResonanceManifold)
			So(ok, ShouldBeTrue)
			layers, surprise, energy := coder.WireSnapshot()
			So(layers, ShouldHaveLength, 3)
			So(layers[0].State, ShouldHaveLength, 3)
			So(layers[1].State, ShouldHaveLength, 6)
			So(layers[2].State, ShouldHaveLength, 3)
			So(math.IsNaN(surprise), ShouldBeFalse)
			So(math.IsNaN(energy), ShouldBeFalse)
		})

		Convey("It should retain the model while the schema is unchanged", func() {
			storedCoder, found := solver.coders.Load("BTC/USD")
			So(found, ShouldBeTrue)

			first = 0.9
			second = -0.1
			appendMeasurement()
			So(solver.Update(thesis), ShouldBeNil)

			updatedCoder, found := solver.coders.Load("BTC/USD")
			So(found, ShouldBeTrue)
			So(updatedCoder, ShouldEqual, storedCoder)
		})
	})

	Convey("Given a nonnegative feature stream with changing observations", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		for _, value := range []float64{0.2, 0.4} {
			value := value
			appendNormalizedMeasurement(symbol, "BTC/USD", types.SourceLiquidity, map[string]*float64{
				"score": &value,
			})

			So(solver.Update(thesis), ShouldBeNil)
			_, found := solver.coders.Load("BTC/USD")
			So(found, ShouldBeFalse)
		}

		belowPriorMean := 0.1
		appendNormalizedMeasurement(symbol, "BTC/USD", types.SourceLiquidity, map[string]*float64{
			"score": &belowPriorMean,
		})
		So(solver.Update(thesis), ShouldBeNil)

		stored, found := solver.coders.Load("BTC/USD")
		So(found, ShouldBeTrue)
		layers, _, _ := stored.(*learning.ResonanceManifold).WireSnapshot()
		So(layers[0].State[0], ShouldBeLessThan, 0)

		abovePriorMean := 0.6
		appendNormalizedMeasurement(symbol, "BTC/USD", types.SourceLiquidity, map[string]*float64{
			"score": &abovePriorMean,
		})
		So(solver.Update(thesis), ShouldBeNil)

		layers, _, _ = stored.(*learning.ResonanceManifold).WireSnapshot()

		Convey("It should center L0 against prior feature moments", func() {
			So(layers[0].State[0], ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a learned schema with a missing feature", t, func() {
		first := 0.1
		second := 0.2
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)
		appendCompleteMeasurement := func() {
			appendNormalizedMeasurement(symbol, "BTC/USD", types.SourceLiquidity, map[string]*float64{
				"first":  &first,
				"second": &second,
			})
		}

		for epoch := range 3 {
			first += float64(epoch) / 10
			second += float64(epoch) / 5
			appendCompleteMeasurement()
			So(solver.Update(thesis), ShouldBeNil)
		}

		stored, found := solver.coders.Load("BTC/USD")
		So(found, ShouldBeTrue)
		before, _, _ := stored.(*learning.ResonanceManifold).WireSnapshot()
		standardizerValue, found := solver.standardizers.Load("liquidity:BTC/USD:first")
		So(found, ShouldBeTrue)
		beforeCount := standardizerValue.(*adaptive.Standardizer).Count()

		first = 0.9
		appendNormalizedMeasurement(symbol, "BTC/USD", types.SourceLiquidity, map[string]*float64{
			"first": &first,
		})
		So(solver.Update(thesis), ShouldBeNil)
		after, _, _ := stored.(*learning.ResonanceManifold).WireSnapshot()

		Convey("It should wait instead of substituting zero", func() {
			So(after[0].State, ShouldResemble, before[0].State)
			So(standardizerValue.(*adaptive.Standardizer).Count(), ShouldEqual, beforeCount)
		})
	})

	Convey("Given normalized measurements for independent symbols", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		symbols := map[string]*types.Symbol{
			"BTC/USD": types.NewSymbol("BTC/USD", nil),
			"ETH/USD": types.NewSymbol("ETH/USD", nil),
		}

		for name, symbol := range symbols {
			thesis.Symbols.Store(name, symbol)
		}

		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		for epoch := range 3 {
			for name, base := range map[string]float64{
				"BTC/USD": 0.25,
				"ETH/USD": -0.25,
			} {
				value := base + float64(epoch)/10
				appendNormalizedMeasurement(symbols[name], name, types.SourceLiquidity, map[string]*float64{
					"score": &value,
				})
			}

			So(solver.Update(thesis), ShouldBeNil)
		}

		Convey("It should retain one manifold per symbol", func() {
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
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		for epoch := range 3 {
			value = 0.25 + float64(epoch)/10

			for source := range types.SignalMetricGroups {
				appendNormalizedMeasurement(symbol, "BTC/USD", source, map[string]*float64{
					"score": &value,
				})
			}

			So(solver.Update(thesis), ShouldBeNil)
		}

		Convey("It should settle the published symbol instead of rejecting its readiness", func() {
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

	Convey("Given enough priced ticks to resolve configured forward horizons", t, func() {
		system.Cfg = system.NewConfig()
		system.Cfg.Resonance.Layers = 3
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		for tick := int64(1); tick <= 6; tick++ {
			value := float64(tick) / 10
			thesis := types.NewThesis(t.Context(), nil)
			symbol := types.NewSymbol("BTC/USD", nil)
			symbol.AppendMeasurement(types.SourceLiquidity, &types.Measurement{
				Source: types.SourceLiquidity,
				Symbol: "BTC/USD",
				Tick:   tick,
				Metadata: map[string]float64{
					"last_price": 100 + float64(tick),
				},
				Metrics: map[string]types.MetricSample{
					"score": {Normalized: &value},
				},
			})
			thesis.Symbols.Store("BTC/USD", symbol)

			So(solver.Update(thesis), ShouldBeNil)
		}

		Convey("It should train one target head per horizon instead of only t+1", func() {
			stored, found := solver.coders.Load("BTC/USD")
			So(found, ShouldBeTrue)
			forecast, err := stored.(*learning.ResonanceManifold).RolloutTaskForecast(1)
			So(err, ShouldBeNil)
			So(forecast, ShouldHaveLength, system.Cfg.Resonance.Layers)

			historyValue, found := solver.histories.Load("BTC/USD")
			So(found, ShouldBeTrue)
			history := historyValue.(*sampleHistory)
			_, retained := history.inputs[1]
			So(retained, ShouldBeFalse)
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
	ctx := b.Context()
	thesis := types.NewThesis(ctx, nil)
	symbol := types.NewSymbol("BTC/USD", nil)
	value := 0.0
	measurement := &types.Measurement{
		Source: types.SourceLiquidity,
		Symbol: "BTC/USD",
		Metrics: map[string]types.MetricSample{
			"second":  {Normalized: &value},
			"first":   {Normalized: &value},
			"third":   {Normalized: &value},
			"fourth":  {Normalized: &value},
			"fifth":   {Normalized: &value},
			"sixth":   {Normalized: &value},
			"seventh": {Normalized: &value},
		},
	}
	thesis.Symbols.Store("BTC/USD", symbol)
	solver := NewSolver(ctx, nil, nil, testAlpha)
	drainOtherConsumers := func() {
		for _, consumer := range []string{"category", "graph", "manifold"} {
			for range symbol.MarketMeasurements(consumer) {
			}
		}
	}

	for sampleIndex := range 3 {
		value = float64(sampleIndex)
		symbol.AppendMeasurement(types.SourceLiquidity, measurement)

		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}

		drainOtherConsumers()
	}

	b.ReportAllocs()
	b.ResetTimer()
	sampleIndex := 3

	for b.Loop() {
		value = float64(sampleIndex)
		symbol.AppendMeasurement(types.SourceLiquidity, measurement)

		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}

		drainOtherConsumers()
		sampleIndex++
	}
}
