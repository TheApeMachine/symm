package resonance

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

const testAlpha = 0.05

func appendResonanceCut(
	symbol *types.Symbol,
	tick int64,
	mark float64,
	value float64,
) {
	for _, source := range types.SignalSources {
		measurement := &types.Measurement{
			Source:   source,
			Symbol:   symbol.Symbol,
			Tick:     tick,
			Metadata: map[string]float64{"last_price": mark},
			Metrics: map[string]types.MetricSample{
				"score": {Normalized: &value},
			},
		}

		symbol.AppendMeasurement(source, measurement)
	}
}

func TestNewSolver(t *testing.T) {
	Convey("Given a configured resonance pace", t, func() {
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		Convey("It should retain the pace and create an empty private model registry", func() {
			So(solver.alpha, ShouldEqual, testAlpha)
			_, found := solver.coders.Load("BTC/USD")
			So(found, ShouldBeFalse)
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Given an invalid learning pace", t, func() {
		solver := NewSolver(t.Context(), nil, nil, -1)

		Convey("It should reject the update", func() {
			So(solver.Update(types.NewThesis(t.Context(), nil)), ShouldNotBeNil)
		})
	})

	Convey("Given distinct model-depth and maximum-horizon settings", t, func() {
		previousConfig := system.Cfg
		system.Cfg = system.NewConfig()
		system.Cfg.Resonance.Layers = 4
		system.Cfg.Resonance.MaxHorizon = 2
		Reset(func() { system.Cfg = previousConfig })

		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store(symbol.Symbol, symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		for tick := int64(1); tick <= 8; tick++ {
			appendResonanceCut(symbol, tick, 100+float64(tick), float64(tick))
			So(solver.Update(thesis), ShouldBeNil)
		}

		Convey("It should publish the nomagique manifold", func() {
			stored, found := symbol.Resonance.Load(symbol.Symbol)
			So(found, ShouldBeTrue)
			coder, valid := stored.(*learning.ResonanceManifold)
			So(valid, ShouldBeTrue)
			layers, _, _ := coder.WireSnapshot()
			forecast, err := coder.RolloutTaskForecast(system.Cfg.Resonance.Layers)
			So(err, ShouldBeNil)
			So(layers, ShouldHaveLength, 3)
			So(forecast, ShouldNotBeEmpty)
		})
	})

	Convey("Given a forecast that later receives its future market price", t, func() {
		previousConfig := system.Cfg
		system.Cfg = system.NewConfig()
		system.Cfg.Resonance.Layers = 3
		system.Cfg.Resonance.MaxHorizon = 4
		Reset(func() { system.Cfg = previousConfig })

		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store(symbol.Symbol, symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)
		var issued learning.RLSOutput
		var issuedMark float64
		var actual float64
		foundIssued := false
		foundResolution := false

		for tick := int64(1); tick <= 40 && !foundResolution; tick++ {
			mark := 100 + float64(tick) + math.Sin(float64(tick))
			appendResonanceCut(symbol, tick, mark, math.Cos(float64(tick)))
			So(solver.Update(thesis), ShouldBeNil)
			stored, found := symbol.Resonance.Load(symbol.Symbol)

			if !found {
				continue
			}

			coder := stored.(*learning.ResonanceManifold)
			forecast, err := coder.RolloutTaskForecast(1)
			So(err, ShouldBeNil)

			if foundIssued {
				actual = math.Log(mark / issuedMark)
				foundResolution = true
				continue
			}

			if len(forecast) > 0 && forecast[0].Ready {
				issued = forecast[0]
				issuedMark = mark
				foundIssued = true
			}
		}

		Convey("It should keep the issued forecast available for later price resolution", func() {
			So(foundIssued, ShouldBeTrue)
			So(foundResolution, ShouldBeTrue)
			So(actual, ShouldNotEqual, 0)
			So(issued.Ready, ShouldBeTrue)
		})
	})

	Convey("Given no normalized measurements", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store(symbol.Symbol, symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		for _, source := range types.SignalSources {
			symbol.AppendMeasurement(source, &types.Measurement{
				Source: source,
				Symbol: symbol.Symbol,
			})
		}

		Convey("It should leave the solver dormant", func() {
			So(solver.Update(thesis), ShouldBeNil)
			_, published := symbol.Resonance.Load(symbol.Symbol)
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
	previousConfig := system.Cfg
	system.Cfg = system.NewConfig()
	system.Cfg.Resonance.Layers = 3
	system.Cfg.Resonance.MaxHorizon = 10
	b.Cleanup(func() { system.Cfg = previousConfig })
	thesis := types.NewThesis(b.Context(), nil)
	symbol := types.NewSymbol("BTC/USD", nil)
	thesis.Symbols.Store(symbol.Symbol, symbol)
	solver := NewSolver(b.Context(), nil, nil, testAlpha)

	for tick := int64(1); tick <= 3; tick++ {
		appendResonanceCut(symbol, tick, 100+float64(tick), float64(tick))
		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()

	for index := range b.N {
		tick := int64(index + 4)
		appendResonanceCut(symbol, tick, 100+float64(tick), math.Sin(float64(tick)))

		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}
	}
}
