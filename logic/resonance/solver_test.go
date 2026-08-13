package resonance

import (
	"context"
	"encoding/json"
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

func appendResonanceSource(
	symbol *types.Symbol,
	source types.SourceType,
	tick int64,
	mark float64,
	value float64,
) {
	symbol.AppendMeasurement(source, &types.Measurement{
		Source: source,
		Symbol: symbol.Symbol,
		Tick:   tick,
		Metadata: map[string]float64{
			"last_price": mark,
		},
		Metrics: map[string]types.MetricSample{
			"score": {Normalized: &value},
		},
	})
}

func TestNewSolver(t *testing.T) {
	Convey("Given a configured resonance pace", t, func() {
		ui := make(chan []byte, 16)
		solver := NewSolver(t.Context(), ui, nil, testAlpha)

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

	Convey("Given a configured model depth and chronological observations", t, func() {
		previousConfig := system.Cfg
		system.Cfg = system.NewConfig()
		system.Cfg.Resonance.Layers = 4
		system.Cfg.Planner.MinimumConfidence = 0.5
		Reset(func() { system.Cfg = previousConfig })

		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store(symbol.Symbol, symbol)
		ui := make(chan []byte, 16)
		solver := NewSolver(t.Context(), ui, nil, testAlpha)

		ticks := []int64{3, 11, 25, 48, 76, 109, 147, 190}

		for epoch, tick := range ticks {
			appendResonanceCut(
				symbol, tick, 100+float64(epoch), float64(epoch+1),
			)
			So(solver.Update(thesis), ShouldBeNil)
		}

		Convey("It should publish the nomagique manifold", func() {
			stored, found := symbol.Resonance.Load(symbol.Symbol)
			So(found, ShouldBeTrue)
			coder, valid := stored.(*learning.ResonanceManifold)
			So(valid, ShouldBeTrue)
			layers, _, _ := coder.WireSnapshot()
			forecast, err := coder.RolloutTaskForecast(3)
			So(err, ShouldBeNil)
			So(layers, ShouldHaveLength, system.Cfg.Resonance.Layers)
			So(forecast, ShouldHaveLength, 3)
			history, found := solver.histories.Load(symbol.Symbol)
			So(found, ShouldBeTrue)
			So(history.(*sampleHistory).resolved, ShouldBeGreaterThan, 0)
			So(history.(*sampleHistory).ledger.resolved, ShouldBeGreaterThan, 0)

			var payload []byte

			for len(ui) > 0 {
				payload = <-ui
			}

			var frame struct {
				Resonance struct {
					TaskScale            *float64 `json:"taskScale"`
					TaskForecast         *float64 `json:"taskForecast"`
					TaskCalibration      string   `json:"taskCalibration"`
					TaskSkillStatus      string   `json:"taskSkillStatus"`
					LastResolvedForecast *float64 `json:"lastResolvedForecast"`
					LastResolvedHorizon  *int     `json:"lastResolvedHorizon"`
					LastRealizedReturn   *float64 `json:"lastRealizedReturn"`
					LastForecastError    *float64 `json:"lastForecastError"`
					Forecast             struct {
						ForwardCurve     []float64 `json:"forwardCurve"`
						SupportedHorizon int       `json:"supportedHorizon"`
						ProbeHorizon     int       `json:"probeHorizon"`
					} `json:"forecast"`
				} `json:"resonance"`
			}
			So(json.Unmarshal(payload, &frame), ShouldBeNil)
			So(frame.Resonance.Forecast.ForwardCurve, ShouldHaveLength,
				frame.Resonance.Forecast.SupportedHorizon)
			So(frame.Resonance.Forecast.ProbeHorizon, ShouldEqual,
				frame.Resonance.Forecast.SupportedHorizon+1)
			So(frame.Resonance.TaskCalibration, ShouldEqual, "calibrated")
			So(frame.Resonance.TaskSkillStatus,
				ShouldBeIn, "above baseline", "baseline", "below baseline")
			So(frame.Resonance.LastResolvedForecast, ShouldNotBeNil)
			So(frame.Resonance.LastResolvedHorizon, ShouldNotBeNil)
			So(*frame.Resonance.LastResolvedHorizon, ShouldBeGreaterThan, 0)
			So(frame.Resonance.LastRealizedReturn, ShouldNotBeNil)
			So(frame.Resonance.LastForecastError, ShouldNotBeNil)
			So(*frame.Resonance.LastForecastError, ShouldAlmostEqual,
				*frame.Resonance.LastRealizedReturn-*frame.Resonance.LastResolvedForecast)

			sequence := history.(*sampleHistory).sequence
			appendResonanceSource(symbol, types.SourceHawkes, 190, 107, 9)
			So(solver.Update(thesis), ShouldBeNil)
			So(history.(*sampleHistory).sequence, ShouldEqual, sequence)
		})
	})

	Convey("Given a forecast that later receives its future market price", t, func() {
		previousConfig := system.Cfg
		system.Cfg = system.NewConfig()
		system.Cfg.Resonance.Layers = 3
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
		strictPriorStored := false
		strictPriorRemoved := false
		resolvedBefore := 0

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
				historyValue, historyFound := solver.histories.Load(symbol.Symbol)
				So(historyFound, ShouldBeTrue)
				history := historyValue.(*sampleHistory)
				foundResolution = history.resolved > resolvedBefore
				_, retained := history.issued[tick-1]
				strictPriorRemoved = !retained
				continue
			}

			if len(forecast) > 0 && forecast[0].Ready {
				issued = forecast[0]
				issuedMark = mark
				historyValue, historyFound := solver.histories.Load(symbol.Symbol)
				So(historyFound, ShouldBeTrue)
				history := historyValue.(*sampleHistory)
				retained, retainedFound := history.issued[tick]
				strictPriorStored = retainedFound &&
					len(retained.features) > 0 &&
					len(retained.prediction) == 1 &&
					retained.prediction[0] == issued.Value
				resolvedBefore = history.resolved
				foundIssued = true
			}
		}

		Convey("It should resolve the exact forecast that was issued", func() {
			So(foundIssued, ShouldBeTrue)
			So(foundResolution, ShouldBeTrue)
			So(strictPriorStored, ShouldBeTrue)
			So(strictPriorRemoved, ShouldBeTrue)
			So(actual, ShouldNotEqual, 0)
			So(issued.Ready, ShouldBeTrue)
		})
	})

	Convey("Given a frontier that issues beyond the currently supported reach", t, func() {
		previousConfig := system.Cfg
		system.Cfg = system.NewConfig()
		system.Cfg.Planner.MinimumConfidence = 0.5
		Reset(func() { system.Cfg = previousConfig })

		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store(symbol.Symbol, symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		for tick := int64(1); tick <= 8; tick++ {
			appendResonanceCut(symbol, tick, 100+float64(tick), float64(tick))
			So(solver.Update(thesis), ShouldBeNil)
		}

		historyValue, found := solver.histories.Load(symbol.Symbol)
		So(found, ShouldBeTrue)
		history := historyValue.(*sampleHistory)
		history.pending = make(map[int64][]issuedHorizon)
		history.ledger = newHorizonLedger()
		history.ledger.observe(1, 0.01, 0.01)
		history.ledger.observe(1, 0.02, 0.02)
		coderValue, coderFound := symbol.Resonance.Load(symbol.Symbol)
		So(coderFound, ShouldBeTrue)
		coder := coderValue.(*learning.ResonanceManifold)
		issued, issuedErr := coder.RolloutTaskForecast(2)
		So(issuedErr, ShouldBeNil)
		So(issued[1].Ready, ShouldBeTrue)

		appendResonanceCut(symbol, 9, 109, 9)
		So(solver.Update(thesis), ShouldBeNil)
		issuedSequence := history.sequence
		pending := history.pending[issuedSequence+2]
		So(pending, ShouldHaveLength, 1)
		So(pending[0].horizon, ShouldEqual, 2)
		issuedForecast := pending[0].forecast

		appendResonanceCut(symbol, 10, 112, 10)
		So(solver.Update(thesis), ShouldBeNil)
		_, prematurelyResolved := history.ledger.horizons[2]

		appendResonanceCut(symbol, 11, 119, 11)
		So(solver.Update(thesis), ShouldBeNil)
		resolved := history.ledger.horizons[2]
		actual := math.Log(119.0 / 112.0)
		expectedAdvantage := actual*actual -
			(actual-issuedForecast)*(actual-issuedForecast)

		Convey("It should resolve tick two only when its exact adjacent outcome arrives", func() {
			So(prematurelyResolved, ShouldBeFalse)
			So(resolved, ShouldNotBeNil)
			So(resolved.count, ShouldEqual, 1)
			So(resolved.mean, ShouldAlmostEqual, expectedAdvantage, 1e-15)
			_, retained := history.pending[issuedSequence+2]
			So(retained, ShouldBeFalse)
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

	Convey("Given normalized sources that update asynchronously", t, func() {
		previousConfig := system.Cfg
		system.Cfg = system.NewConfig()
		system.Cfg.Resonance.Layers = 3
		Reset(func() { system.Cfg = previousConfig })

		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store(symbol.Symbol, symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		for tick := int64(1); tick <= 4; tick++ {
			appendResonanceSource(
				symbol, types.SourceHawkes, tick, 100+float64(tick), float64(tick),
			)
			So(solver.Update(thesis), ShouldBeNil)
		}

		Convey("It should settle before every configured signal has emitted", func() {
			activeCoder, found := symbol.Resonance.Load(symbol.Symbol)
			So(found, ShouldBeTrue)
			activeHistory, found := solver.histories.Load(symbol.Symbol)
			So(found, ShouldBeTrue)
			resolvedBefore := activeHistory.(*sampleHistory).resolved
			So(resolvedBefore, ShouldBeGreaterThan, 0)

			for tick := int64(5); tick <= 7; tick++ {
				appendResonanceSource(
					symbol, types.SourceCVD, tick, 100+float64(tick), float64(tick),
				)
				So(solver.Update(thesis), ShouldBeNil)
			}

			appendResonanceSource(symbol, types.SourceHawkes, 8, 108, 8)
			So(solver.Update(thesis), ShouldBeNil)
			schema, found := solver.schemas.Load(symbol.Symbol)
			So(found, ShouldBeTrue)
			So(schema, ShouldHaveLength, 1)
			retainedCoder, found := symbol.Resonance.Load(symbol.Symbol)
			So(found, ShouldBeTrue)
			So(retainedCoder, ShouldEqual, activeCoder)
			retainedHistory, found := solver.histories.Load(symbol.Symbol)
			So(found, ShouldBeTrue)
			So(retainedHistory.(*sampleHistory).resolved,
				ShouldBeGreaterThan, resolvedBefore)
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
