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
				types.MetricKey(types.MetricHypothesisSeparation, types.SideNone): {
					Normalized: &value,
				},
			},
		}

		symbol.AppendMeasurements([]*types.Measurement{measurement})
	}
}

func appendResonanceSource(
	symbol *types.Symbol,
	source types.SourceType,
	tick int64,
	mark float64,
	value float64,
) {
	symbol.AppendMeasurements([]*types.Measurement{&types.Measurement{
		Source: source,
		Symbol: symbol.Symbol,
		Tick:   tick,
		Metadata: map[string]float64{
			"last_price": mark,
		},
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricHypothesisSeparation, types.SideNone): {
				Normalized: &value,
			},
		},
	}})
}

func TestTaskSchema(t *testing.T) {
	Convey("Given the predictive direction task", t, func() {
		schema := taskSchema("BTC/USD")
		hypothesis := string(types.SourceCVD) + ":BTC/USD:" +
			types.MetricKey(types.MetricHypothesisSeparation, types.SideNone)
		rawPrice := string(types.SourceCVD) + ":BTC/USD:" +
			types.MetricKey(types.MetricTradePrice, types.SideNone)

		Convey("It should fix semantic evidence and universe context without raw price coordinates", func() {
			_, hasHypothesis := schema.known[hypothesis]
			_, hasRawPrice := schema.known[rawPrice]
			_, hasManifold := schema.known[manifoldContextIdentity("BTC/USD", "coherence")]
			So(hasHypothesis, ShouldBeTrue)
			So(hasRawPrice, ShouldBeFalse)
			So(hasManifold, ShouldBeTrue)
			So(len(schema.known), ShouldEqual, len(schema.identities))
		})
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
			thesis.Tick = tick
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
					TaskDirection *float64 `json:"taskDirection"`
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

			sequence := history.(*sampleHistory).sequence
			thesis.Tick = 190
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
			thesis.Tick = tick
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
					retained.prediction[0] == signedDirection(issued.Value)
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
			thesis.Tick = tick
			appendResonanceCut(symbol, tick, 100+float64(tick), float64(tick))
			So(solver.Update(thesis), ShouldBeNil)
		}

		historyValue, found := solver.histories.Load(symbol.Symbol)
		So(found, ShouldBeTrue)
		history := historyValue.(*sampleHistory)
		history.pending = make(map[int64][]issuedHorizon)
		history.ledger = newHorizonLedger()
		history.ledger.observe(1, 1, 0.01)
		history.ledger.observe(1, 1, 0.02)
		coderValue, coderFound := symbol.Resonance.Load(symbol.Symbol)
		So(coderFound, ShouldBeTrue)
		coder := coderValue.(*learning.ResonanceManifold)
		issued, issuedErr := coder.RolloutTaskForecast(2)
		So(issuedErr, ShouldBeNil)
		So(issued[1].Ready, ShouldBeTrue)

		thesis.Tick = 9
		appendResonanceCut(symbol, 9, 109, 9)
		So(solver.Update(thesis), ShouldBeNil)
		issuedSequence := history.sequence
		pending := history.pending[issuedSequence+2]
		So(pending, ShouldHaveLength, 1)
		So(pending[0].horizon, ShouldEqual, 2)
		issuedForecast := pending[0].forecast

		thesis.Tick = 10
		appendResonanceCut(symbol, 10, 112, 10)
		So(solver.Update(thesis), ShouldBeNil)
		_, prematurelyResolved := history.ledger.horizons[2]

		thesis.Tick = 11
		appendResonanceCut(symbol, 11, 119, 11)
		So(solver.Update(thesis), ShouldBeNil)
		resolved := history.ledger.horizons[2]
		actual := math.Log(119.0 / 109.0)
		expectedAdvantage := signedDirection(issuedForecast) * signedDirection(actual)

		Convey("It should resolve the two-step move only when that later mark arrives", func() {
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
			symbol.AppendMeasurements([]*types.Measurement{&types.Measurement{
				Source: source,
				Symbol: symbol.Symbol,
			}})
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
			thesis.Tick = tick
			appendResonanceSource(
				symbol, types.SourceHawkes, tick, 100+float64(tick), float64(tick),
			)
			So(solver.Update(thesis), ShouldBeNil)
		}

		Convey("It should keep one stable semantic schema as late sources arrive", func() {
			activeCoder, found := symbol.Resonance.Load(symbol.Symbol)
			So(found, ShouldBeTrue)
			So(activeCoder, ShouldNotBeNil)
			historyValue, historyFound := solver.histories.Load(symbol.Symbol)
			So(historyFound, ShouldBeTrue)
			history := historyValue.(*sampleHistory)
			ledger := history.ledger
			resolvedBefore := history.ledger.resolved

			for tick := int64(5); tick <= 12; tick++ {
				thesis.Tick = tick
				appendResonanceSource(
					symbol, types.SourceHawkes, tick, 100+float64(tick), float64(tick),
				)
				appendResonanceSource(
					symbol, types.SourceCVD, tick, 100+float64(tick), float64(tick),
				)
				So(solver.Update(thesis), ShouldBeNil)
			}

			schemaValue, schemaFound := solver.schemas.Load(symbol.Symbol)
			So(schemaFound, ShouldBeTrue)
			So(
				schemaValue.(*featureSchema).identities,
				ShouldResemble,
				taskSchema(symbol.Symbol).identities,
			)
			retainedCoder, coderFound := symbol.Resonance.Load(symbol.Symbol)
			So(coderFound, ShouldBeTrue)
			So(retainedCoder, ShouldEqual, activeCoder)
			So(history.ledger, ShouldEqual, ledger)
			So(history.ledger.resolved, ShouldBeGreaterThanOrEqualTo, resolvedBefore)
		})
	})

	Convey("Given delayed signal rows from consecutive ticker epochs", t, func() {
		previousConfig := system.Cfg
		system.Cfg = system.NewConfig()
		system.Cfg.Resonance.Layers = 3
		Reset(func() { system.Cfg = previousConfig })

		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("SYND/USD", nil)
		thesis.Symbols.Store(symbol.Symbol, symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		for analysisTick := int64(1); analysisTick <= 8; analysisTick++ {
			thesis.Tick = analysisTick
			appendResonanceSource(
				symbol,
				types.SourceCVD,
				1,
				0.015+float64(analysisTick)*0.0001,
				float64(analysisTick),
			)
			So(solver.Update(thesis), ShouldBeNil)
		}

		Convey("It should resolve each informative analysis epoch instead of freezing on the row tick", func() {
			historyValue, found := solver.histories.Load(symbol.Symbol)
			So(found, ShouldBeTrue)
			history := historyValue.(*sampleHistory)
			So(history.sequence, ShouldEqual, 6)
			So(history.resolved, ShouldEqual, 5)
			So(history.ticks, ShouldResemble, []int64{8})
		})
	})

	Convey("Given an initialized model followed by ticker-only market marks", t, func() {
		previousConfig := system.Cfg
		system.Cfg = system.NewConfig()
		system.Cfg.Resonance.Layers = 3
		Reset(func() { system.Cfg = previousConfig })

		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("AKE/USD", nil)
		thesis.Symbols.Store(symbol.Symbol, symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		for tick := int64(1); tick <= 12; tick++ {
			thesis.Tick = tick
			appendResonanceSource(
				symbol,
				types.SourceCVD,
				tick,
				100+float64(tick),
				math.Sin(float64(tick)),
			)
			So(solver.Update(thesis), ShouldBeNil)
		}

		coderValue, found := solver.coders.Load(symbol.Symbol)
		So(found, ShouldBeTrue)
		coder := coderValue.(*learning.ResonanceManifold)
		before, err := coder.RolloutTaskForecast(1)
		So(err, ShouldBeNil)
		So(before, ShouldHaveLength, 1)
		So(before[0].Ready, ShouldBeTrue)
		historyValue, found := solver.histories.Load(symbol.Symbol)
		So(found, ShouldBeTrue)
		history := historyValue.(*sampleHistory)
		sequenceBefore := history.sequence
		resolvedBefore := history.resolved

		for tick := int64(13); tick <= 16; tick++ {
			thesis.Tick = tick
			So(symbol.AppendResonanceMeasurement(&types.ResonanceMeasurement{
				Tick: tick,
				Mark: 112 + float64(tick-12)*2,
			}), ShouldBeTrue)
			So(solver.Update(thesis), ShouldBeNil)
		}

		after, err := coder.RolloutTaskForecast(1)
		So(err, ShouldBeNil)
		So(after, ShouldHaveLength, 1)

		Convey("It should resolve issued forecasts and keep learning from every next tick", func() {
			So(history.sequence, ShouldEqual, sequenceBefore+4)
			So(history.resolved, ShouldEqual, resolvedBefore+4)
			So(after[0].DegreesOfFreedom,
				ShouldBeGreaterThan, before[0].DegreesOfFreedom)
		})
	})

	Convey("Given a consistently signed price path", t, func() {
		previousConfig := system.Cfg
		system.Cfg = system.NewConfig()
		system.Cfg.Resonance.Layers = 3
		system.Cfg.Planner.MinimumConfidence = 0.5
		Reset(func() { system.Cfg = previousConfig })

		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store(symbol.Symbol, symbol)
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		for tick := int64(1); tick <= 16; tick++ {
			thesis.Tick = tick
			appendResonanceCut(symbol, tick, 100+float64(tick), float64(tick))
			So(solver.Update(thesis), ShouldBeNil)
		}

		stored, found := symbol.Resonance.Load(types.ResonanceReturnForecastKey)
		historyValue, historyFound := solver.histories.Load(symbol.Symbol)

		Convey("It should call a direction and extend reach past one tick", func() {
			So(found, ShouldBeTrue)
			So(historyFound, ShouldBeTrue)
			forecast := stored.(*types.ResonanceReturnForecast)
			So(forecast.Call, ShouldNotEqual, 0)
			So(forecast.Horizon, ShouldBeGreaterThan, 1)
			So(historyValue.(*sampleHistory).ledger.supported(0.5),
				ShouldBeGreaterThan, 1)
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
		thesis.Tick = tick
		appendResonanceCut(symbol, tick, 100+float64(tick), float64(tick))
		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()

	for index := range b.N {
		tick := int64(index + 4)
		thesis.Tick = tick
		appendResonanceCut(symbol, tick, 100+float64(tick), math.Sin(float64(tick)))

		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}
	}
}
