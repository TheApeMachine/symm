package logic

import (
	"strings"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDecisionMeasure(testingTB *testing.T) {
	Convey("Given a decision engine owning the sequential ladder", testingTB, func() {
		restoreDecisionConfig()
		decision := NewDecision(nil)
		defer decision.Close()
		at := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

		Convey("When a staged source is observed", func() {
			action, err := decision.Observe(1, stagedMeasurement("BTC/USD", at))

			Convey("Then it should reject the non-signal input", func() {
				So(action, ShouldBeNil)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "staged sources")
			})
		})

		Convey("When one normal observation reaches the ladder", func() {
			action, err := decision.Observe(1, hawkesMeasurement("BTC/USD", at, 0))

			Convey("Then Pearl should wait for causal history", func() {
				So(err, ShouldBeNil)
				So(action, ShouldBeNil)
			})
		})

		Convey("When one observation has an unusable boundary schema", func() {
			measurement := hawkesMeasurement("BTC/USD", at, 0)
			measurement.Metrics = map[string]float64{
				"branchingRatio": 0.4,
			}
			batch, err := decision.Measure([]*types.Measurement{measurement})

			Convey("Then the decision batch should continue without an action", func() {
				So(err, ShouldBeNil)
				So(batch.Actions, ShouldHaveLength, 0)
			})
		})

		Convey("When cognitive priors are applied to the event runtime", func() {
			err := decision.SetPrior("BTC/USD", DecisionPrior{
				TopdownPhaseScale:  0.25,
				TopdownEnergyScale: 0.5,
			})
			state := decision.state("BTC/USD")
			runtime, runtimeErr := decision.runtime(1, "BTC/USD", state)
			advanceErr := runtime.Advance(at)
			controls, controlsErr := runtime.controls()

			Convey("Then the solver controls should carry the prior and event interval", func() {
				So(err, ShouldBeNil)
				So(runtimeErr, ShouldBeNil)
				So(advanceErr, ShouldBeNil)
				So(controlsErr, ShouldBeNil)
				So(controls.DeltaT, ShouldEqual, 0.1)
				So(controls.MetabolicRate, ShouldEqual, 10)
				So(controls.TopdownPhaseScale, ShouldEqual, 0.25)
				So(controls.TopdownEnergyScale, ShouldEqual, 0.5)
			})
		})

		Convey("When sparse events advance the decision clock", func() {
			state := decision.state("BTC/USD")
			first, firstErr := decision.runtime(1, "BTC/USD", state)
			firstAdvanceErr := first.Advance(at)
			runtime, secondErr := decision.runtime(2, "BTC/USD", state)
			secondAdvanceErr := runtime.Advance(at.Add(350 * time.Millisecond))

			Convey("Then virtual clicks should fill the gap without exceeding the integration step", func() {
				So(firstErr, ShouldBeNil)
				So(firstAdvanceErr, ShouldBeNil)
				So(secondErr, ShouldBeNil)
				So(secondAdvanceErr, ShouldBeNil)
				So(runtime.DeltaT, ShouldEqual, 100*time.Millisecond)
				So(state.clock.Click(), ShouldEqual, int64(4))
			})
		})

		Convey("When an older frame arrives after a newer frame", func() {
			state := decision.state("BTC/USD")
			first, firstErr := decision.runtime(1, "BTC/USD", state)
			firstAdvanceErr := first.Advance(at.Add(time.Second))
			runtime, secondErr := decision.runtime(2, "BTC/USD", state)
			secondAdvanceErr := runtime.Advance(at)

			Convey("Then the clock should not move backwards or reject the frame", func() {
				So(firstErr, ShouldBeNil)
				So(firstAdvanceErr, ShouldBeNil)
				So(secondErr, ShouldBeNil)
				So(secondAdvanceErr, ShouldBeNil)
				So(runtime.DeltaT, ShouldEqual, 100*time.Millisecond)
				So(state.clock.Click(), ShouldEqual, int64(1))
				So(state.lastEventAt, ShouldEqual, at.Add(time.Second))
			})
		})

		Convey("When an older measurement for the same source arrives late", func() {
			newer := hawkesMeasurement("BTC/USD", at.Add(time.Second), 1)
			older := hawkesMeasurement("BTC/USD", at, 0)
			_, newerErr := decision.Observe(1, newer)
			_, olderErr := decision.Observe(2, older)
			state := decision.state("BTC/USD")

			Convey("Then it should not replace the newer source state", func() {
				So(newerErr, ShouldBeNil)
				So(olderErr, ShouldBeNil)
				So(state.measurements[types.SourceHawkes].At, ShouldEqual, newer.At)
			})
		})

		Convey("When normal measurements build enough physical history", func() {
			action, err := observeCascade(decision, "BTC/USD", at)

			Convey("Then the ladder should emit a physical predictive causal action", func() {
				So(err, ShouldBeNil)
				So(action, ShouldNotBeNil)
				So(action.Symbol, ShouldEqual, "BTC/USD")
				So(action.BranchKey, ShouldNotEqual, "")
				So(action.EntryScore, ShouldBeGreaterThan, 0)
				So(action.EntryConfidence, ShouldBeGreaterThan, 0)
				So(action.DecisionAt, ShouldNotEqual, "")
				So(action.Price, ShouldBeGreaterThan, 0)
			})
		})

		Convey("When normal measurements are measured for the UI", func() {
			batch, err := decision.Measure(normalMeasurements("BTC/USD", at, 0))

			Convey("Then manifold and resonance frames should be emitted before actions", func() {
				So(err, ShouldBeNil)
				So(batch.Manifold, ShouldNotBeEmpty)
				So(batch.Resonance, ShouldNotBeEmpty)
				So(batch.Manifold[0].Source, ShouldEqual, types.SourceManifold)
				So(batch.Manifold[0].Symbol, ShouldEqual, "BTC/USD")
				So(batch.Manifold[0].Rho, ShouldNotBeEmpty)
				So(batch.Resonance[0].Source, ShouldEqual, types.SourceResonance)
				So(batch.Resonance[0].Symbol, ShouldEqual, "BTC/USD")
				So(batch.Resonance[0].Latent, ShouldNotBeEmpty)
			})
		})

		Convey("When positive evidence is actioned", func() {
			action := decision.action(1, "BTC/USD", actionEvidence(1))

			Convey("Then the action should be a buy entry", func() {
				So(action.Verdict, ShouldEqual, "allow")
				So(action.Type, ShouldEqual, "entry")
				So(action.Side, ShouldEqual, "buy")
				So(action.Fraction, ShouldBeGreaterThan, 0)
			})
		})

		Convey("When negative evidence is actioned", func() {
			action := decision.action(1, "BTC/USD", actionEvidence(-1))

			Convey("Then the action should be a sell exit", func() {
				So(action.Verdict, ShouldEqual, "allow")
				So(action.Type, ShouldEqual, "exit")
				So(action.Side, ShouldEqual, "sell")
			})
		})
	})
}

func BenchmarkDecisionMeasure(benchmarkTB *testing.B) {
	restoreDecisionConfig()

	decision := NewDecision(nil)
	defer decision.Close()
	at := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	_, _ = observeCascade(decision, "BTC/USD", at)
	measurements := normalMeasurements("BTC/USD", at.Add(12*time.Second), 12)

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		if _, err := decision.Measure(measurements); err != nil {
			benchmarkTB.Fatal(err)
		}
	}
}

func restoreDecisionConfig() {
	viper.Reset()
	viper.Set("trading.sizing.base_fraction", 0.10)
	viper.Set("market.story.measurement_max_age", time.Minute)
	viper.Set("market.book.depth", 8)
	viper.Set("telemetry.gauge.publish_interval", 100*time.Millisecond)
}

func observeCascade(
	decision *Decision,
	symbol string,
	at time.Time,
) (*Action, error) {
	var action *Action
	var err error

	for step := 0; step < 8; step++ {
		for _, measurement := range normalMeasurements(
			symbol,
			at.Add(time.Duration(step)*time.Second),
			step,
		) {
			action, err = decision.Observe(int64(step+1), measurement)
			if err != nil {
				return nil, err
			}
		}
	}

	return action, nil
}

func normalMeasurements(
	symbol string,
	at time.Time,
	step int,
) []*types.Measurement {
	return []*types.Measurement{
		hawkesMeasurement(symbol, at, step),
		cvdMeasurement(symbol, at, step),
		fluidMeasurement(symbol, at, step),
		liquidityMeasurement(symbol, at, step),
	}
}

func hawkesMeasurement(symbol string, at time.Time, step int) *types.Measurement {
	frenzy := 0.30 + float64(step)*0.02
	organic := 0.20 + float64(step)*0.01

	return &types.Measurement{
		Source:        types.SourceHawkes,
		Symbol:        symbol,
		At:            at,
		EntryBaseline: 0.2,
		ExitBaseline:  0.1,
		Categories: []types.Category{
			{Type: types.CategoryFrenzy, Confidence: 0.6, Strength: frenzy},
			{Type: types.CategoryOrganic, Confidence: 0.4, Strength: organic},
		},
		Metrics: map[string]float64{
			"branchingRatio":     frenzy,
			"intensityRatio":     organic,
			"spectralRadius":     frenzy + organic,
			"stationarityMargin": organic,
			"baselineMu":         organic,
			"strength":           frenzy,
		},
	}
}

func cvdMeasurement(symbol string, at time.Time, step int) *types.Measurement {
	drive := 0.34 + float64(step)*0.02
	absorption := 0.12 + float64(step)*0.005
	balance := 0.22 + float64(step)*0.01
	starvation := 0.08 + float64(step)*0.004

	return &types.Measurement{
		Source:        types.SourceCVD,
		Symbol:        symbol,
		At:            at,
		EntryBaseline: 0.2,
		ExitBaseline:  0.1,
		Categories: []types.Category{
			{Type: types.CategoryAggressiveDrive, Confidence: 0.7, Strength: drive},
			{Type: types.CategoryHiddenAbsorption, Confidence: 0.3, Strength: absorption},
		},
		Metrics: map[string]float64{
			"absorption": absorption,
			"drive":      drive,
			"balance":    balance,
			"starvation": starvation,
			"strength":   drive,
		},
	}
}

func fluidMeasurement(symbol string, at time.Time, step int) *types.Measurement {
	laminar := 0.40 + float64(step)*0.02
	turbulent := 0.12 + float64(step)*0.01
	inertial := 0.25 + float64(step)*0.02
	viscous := 0.10 + float64(step)*0.005

	return &types.Measurement{
		Source:        types.SourceFluid,
		Symbol:        symbol,
		At:            at,
		EntryBaseline: 0.2,
		ExitBaseline:  0.1,
		Categories: []types.Category{
			{Type: types.CategoryLaminar, Confidence: 0.5, Strength: laminar},
			{Type: types.CategoryInertial, Confidence: 0.3, Strength: inertial},
			{Type: types.CategoryTurbulent, Confidence: 0.2, Strength: turbulent},
		},
		Metrics: map[string]float64{
			"price":      100 + float64(step),
			"reynolds":   inertial,
			"vorticity":  turbulent,
			"viscosity":  viscous,
			"divergence": laminar,
			"turbulence": turbulent,
			"memory":     viscous,
			"strength":   laminar,
		},
	}
}

func liquidityMeasurement(symbol string, at time.Time, step int) *types.Measurement {
	robust := 0.35 + float64(step)*0.015
	scarcity := 0.10 + float64(step)*0.005
	median := 0.25 + float64(step)*0.01

	return &types.Measurement{
		Source:        types.SourceLiquidity,
		Symbol:        symbol,
		At:            at,
		EntryBaseline: 0.2,
		ExitBaseline:  0.1,
		Categories: []types.Category{
			{Type: types.CategoryRobustLiquidity, Confidence: 0.6, Strength: robust},
			{Type: types.CategoryMedianDepth, Confidence: 0.3, Strength: median},
			{Type: types.CategoryExtremeScarcity, Confidence: 0.1, Strength: scarcity},
		},
		Metrics: map[string]float64{
			"scarcityScore": scarcity,
			"medianScore":   median,
			"depthScore":    robust,
			"strength":      robust,
		},
	}
}

func stagedMeasurement(symbol string, at time.Time) *types.Measurement {
	return &types.Measurement{
		Source:        types.SourceManifold,
		Symbol:        strings.TrimSpace(symbol),
		At:            at,
		EntryBaseline: 0.2,
		ExitBaseline:  0.1,
		Categories: []types.Category{
			{Type: types.CategorySystemicHerd, Confidence: 0.8, Strength: 0.5},
		},
		Metrics: map[string]float64{"strength": 0.5},
	}
}

func actionEvidence(momentum float64) decisionEvidence {
	return decisionEvidence{
		physical: physicalEvidence{
			category: types.CategoryPhysicalField,
			rho: rhoEvidence{
				mass: 1,
			},
		},
		predictive: predictiveEvidence{
			category:   types.CategoryLaminarResonance,
			confidence: 0.8,
			flow:       2,
			stress:     0.5,
			coupling:   0.4,
			baseline:   1,
		},
		counterfactual: counterfactualEvidence{
			category:     types.CategoryEndogenousAlpha,
			confidence:   0.7,
			strength:     2,
			baseline:     1,
			uplift:       1,
			intervention: 1,
			beta:         0.1,
			panic:        0.1,
			residual:     0.1,
		},
		price:    *decimal.NewFromFloat64(100),
		momentum: momentum,
		pressure: 1,
		at:       time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}
}
