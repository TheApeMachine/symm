package strategy

import (
	"math"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestCognitionFactor(t *testing.T) {
	Convey("Given ready cognition with attractor basin confidence", t, func() {
		cognition := types.Cognition{
			Ready:          true,
			Confidence:     0.8,
			LookaheadPaths: 1,
			LookaheadScore: math.Log(0.25),
		}

		Convey("It should scale the forecast by basin confidence without adding direction", func() {
			factor := cognitionFactor(cognition)

			So(factor, ShouldEqual, 0.8)
			So(factor, ShouldBeGreaterThan, 0.0)
			So(factor, ShouldBeLessThanOrEqualTo, 1.0)
		})

		Convey("It should leave the forecast whole when confidence is 1.0", func() {
			cognition.Confidence = 1.0

			So(cognitionFactor(cognition), ShouldEqual, 1.0)
		})
	})

	Convey("Given cognition that is not ready", t, func() {
		cognition := types.Cognition{
			Confidence:     0.8,
			LookaheadPaths: 1,
			LookaheadScore: math.Log(0.25),
		}

		Convey("It should not alter the forecast", func() {
			So(cognitionFactor(cognition), ShouldEqual, 1.0)
		})
	})
}

func TestUnifiedUtility(t *testing.T) {
	Convey("Given a forecast of nothing", t, func() {
		Convey("It should stay worth nothing however corroborated it is", func() {
			So(unifiedUtility(0, 2, 1, 1, 0), ShouldEqual, 0.0)
		})
	})

	Convey("Given corroborating heads and Bayesian precision weighting", t, func() {
		Convey("It should discount uncertain forecasts gracefully without inverting positive returns", func() {
			So(uncertaintyWeight(0), ShouldEqual, 1.0)
			So(uncertaintyWeight(1), ShouldEqual, 0.5)
			So(uncertaintyWeight(math.NaN()), ShouldEqual, 1.0)

			So(
				unifiedUtility(0.01, causalFactor(0, 0, 0, false), cognitionFactor(types.Cognition{}), graphFactor(0, 0, false), 1.0),
				ShouldAlmostEqual,
				0.005,
			)
		})
	})

	Convey("Given a graph that wholly contradicts the trade", t, func() {
		Convey("It should leave no return", func() {
			So(graphFactor(0, 3, true), ShouldEqual, 0.0)
			So(unifiedUtility(0.01, 1, 1, graphFactor(0, 3, true), 0), ShouldEqual, 0.0)
		})
	})

	Convey("Given a causal head arguing against the forecast", t, func() {
		Convey("It should discount the return toward nothing without inverting it", func() {
			opposed := causalFactor(-2, 0, 0, true)

			So(opposed, ShouldBeGreaterThan, 0.0)
			So(opposed, ShouldBeLessThan, 1.0)

			So(unifiedUtility(0.01, opposed, 1, 1, 0), ShouldBeGreaterThan, 0.0)
			So(unifiedUtility(0.01, opposed, 1, 1, 0), ShouldBeLessThan, 0.01)
		})
	})
}

func TestGetCausalHistoryRows(t *testing.T) {
	Convey("Given a causal output without aligned history", t, func() {
		thesis := types.NewThesis()
		thesis.Causal.Store("BTC/USD", map[string]any{
			"intervention":  0.2,
			"doExpectation": 0.1,
		})

		Convey("It should not fabricate a causal row from unrelated output fields", func() {
			So(getCausalHistoryRows(thesis, "BTC/USD"), ShouldBeNil)
		})
	})

	Convey("Given aligned causal history", t, func() {
		thesis := types.NewThesis()
		rows := [][]float64{{0.1, 0.2, 0.3, 0.4}}
		thesis.Causal.Store("BTC/USD", map[string]any{"historyRows": rows})

		Convey("It should return the real aligned rows", func() {
			So(getCausalHistoryRows(thesis, "BTC/USD"), ShouldResemble, rows)
		})
	})
}

func TestProjectedReturn(t *testing.T) {
	Convey("Given the ZRO resonance rollout observed in the paper run", t, func() {
		curve := []float64{
			0.0002459297267317184, 0.00024366941030828297,
			0.00024525064080171835, 0.00024909273459577756,
			0.0002540490272959324, 0.0002595214621685936,
			0.00026521743482255344, 0.00027094679624451665,
			0.00027657830320614, 0.00028201653462815593,
			0.00028719244808729917,
		}
		surviving := []float64{
			0.9055048878804701, 0.8974399798115656,
			0.9093932877015342, 0.9296458889374464,
			0.9536892103079515, 0.9792906095758916,
			1.0053454119173224, 1.0311457162739686,
			1.0562061772730253, 1.080184430729682,
			1.1028428817730502,
		}

		Convey("It should preserve the rollout magnitude instead of exponentiating signal scores", func() {
			projected := projectedReturn(curve, surviving)

			So(projected, ShouldAlmostEqual,
				math.Expm1(accumulatedReturn(curve, surviving)), 1e-12)
			So(projected, ShouldBeGreaterThan, 0.002)
			So(projected, ShouldBeLessThan, 0.004)
		})
	})
}

func TestEstimateImpact(t *testing.T) {
	Convey("Given market spread and scarcity measurements", t, func() {
		thesis := types.NewThesis()
		symbol := "BTC/USD"
		spread := decimal.NewFromFloat64(10.0)

		Convey("With baseline liquidity, impact should be derived from baseline touch", func() {
			impact := estimateImpact(thesis, symbol, spread)
			So(impact.Float64(), ShouldAlmostEqual, 0.5) // 10.0 * 0.05
		})

		Convey("With scarcity measurements, impact should dynamically scale up", func() {
			scarcitySample := types.MetricSample{Raw: 0.8}
			measurement := &types.Measurement{
				Source: types.SourceLiquidity,
				Symbol: symbol,
				Metrics: map[string]types.MetricSample{
					string(types.MetricKey(types.MetricScarcityScore, types.SideNone)): scarcitySample,
				},
			}
			thesis.Measurements.Store(string(types.SourceLiquidity), []*types.Measurement{measurement})

			impact := estimateImpact(thesis, symbol, spread)
			So(impact.Float64(), ShouldBeGreaterThan, 0.5)
			So(impact.Float64(), ShouldAlmostEqual, 2.5) // 10.0 * (0.05 + 0.25*0.8) = 2.5
		})
	})
}

func TestExhaustionHoldDiscount(t *testing.T) {
	Convey("Given valid long-side exhaustion output", t, func() {
		thesis := types.NewThesis()

		thesis.Measurements.Store(types.SourceExhaustion, []*types.Measurement{{
			Source: types.SourceExhaustion,
			Symbol: "BTC/USD",
			Validity: types.MeasurementValidity{
				State: types.ValidityValid,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricUrgency, types.SideBuy): {Raw: 0.8},
			},
		}})

		discount, ready := exhaustionHoldDiscount(thesis, "BTC/USD")

		Convey("It should convert the strongest live decay hazard to survival", func() {
			So(ready, ShouldBeTrue)
			So(discount, ShouldAlmostEqual, math.Exp(-0.8), 1e-12)
			So(discount, ShouldBeLessThan, 1.0)
			So(discount, ShouldBeGreaterThan, 0.0)
		})

		Convey("It should refuse a symbol without its own decay reading", func() {
			_, found := exhaustionHoldDiscount(thesis, "ETH/USD")
			So(found, ShouldBeFalse)
		})

		Convey("A clean reading should remain strictly below one", func() {
			measurement, found := latestMeasurement(
				thesis, "BTC/USD", types.SourceExhaustion,
			)
			So(found, ShouldBeTrue)
			measurement.Metrics[types.MetricKey(
				types.MetricUrgency, types.SideBuy,
			)] = types.MetricSample{Raw: 0}

			discount, ready := exhaustionHoldDiscount(thesis, "BTC/USD")
			So(ready, ShouldBeTrue)
			So(discount, ShouldBeLessThan, 1.0)
			So(discount, ShouldBeGreaterThan, 0.0)
		})
	})
}

func TestRegimeExit(t *testing.T) {
	Convey("Given a valid empirically scaled pump-dump reading", t, func() {
		thesis := types.NewThesis()
		thesis.Measurements.Store(types.SourcePumpDump, []*types.Measurement{{
			Source: types.SourcePumpDump,
			Symbol: "BTC/USD",
			Validity: types.MeasurementValidity{
				State: types.ValidityValid,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricIgnition, types.SideBuy):  {Raw: 0.4},
				types.MetricKey(types.MetricIgnition, types.SideSell): {Raw: 1.2},
			},
		}})

		Convey("A downward ignition above its empirical baseline should exit", func() {
			So(regimeExit(thesis, "BTC/USD"), ShouldEqual,
				types.TriggerPumpDumpSellIgnition)
		})

		Convey("A downward ignition below its empirical baseline should not exit", func() {
			measurement, found := latestMeasurement(
				thesis, "BTC/USD", types.SourcePumpDump,
			)
			So(found, ShouldBeTrue)
			measurement.Metrics[types.MetricKey(
				types.MetricIgnition, types.SideSell,
			)] = types.MetricSample{Raw: 0.8}

			So(regimeExit(thesis, "BTC/USD"), ShouldBeBlank)
		})
	})

	Convey("Given a fitted seller-dominant Hawkes process", t, func() {
		thesis := types.NewThesis()
		thesis.Measurements.Store(types.SourceHawkes, []*types.Measurement{{
			Source: types.SourceHawkes,
			Symbol: "BTC/USD",
			Validity: types.MeasurementValidity{
				State:     types.ValidityProvisional,
				Readiness: types.ReadinessModel,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricSpectralRadius, types.SideNone):       {Raw: 0.9},
				types.MetricKey(types.MetricTotalDescendants, types.SideBuy):      {Raw: 0.5},
				types.MetricKey(types.MetricTotalDescendants, types.SideSell):     {Raw: 1.4},
				types.MetricKey(types.MetricConditionalIntensity, types.SideBuy):  {Raw: 2},
				types.MetricKey(types.MetricConditionalIntensity, types.SideSell): {Raw: 5},
			},
		}})

		Convey("A sell parent expected to reproduce should exit", func() {
			So(regimeExit(thesis, "BTC/USD"), ShouldEqual,
				types.TriggerHawkesSellCascade)
		})
	})
}

func TestAllocationHaircut(t *testing.T) {
	Convey("Given valid executable-liquidity and toxicity evidence", t, func() {
		thesis := types.NewThesis()
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Validity: types.MeasurementValidity{
				State: types.ValidityValid,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricScarcityScore, types.SideNone): {Raw: 0.5},
			},
		}})
		thesis.Measurements.Store(types.SourceToxicity, []*types.Measurement{{
			Source: types.SourceToxicity,
			Symbol: "BTC/USD",
			Validity: types.MeasurementValidity{
				State: types.ValidityValid,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricCancelledQuantity, types.SideBuy): {Raw: 4},
				types.MetricKey(types.MetricTouchQuantity, types.SideBuy):     {Raw: 6},
			},
		}})
		forecast := candidate{
			Symbol:         "BTC/USD",
			ExpectedSpread: decimal.NewFromFloat64(0.2),
		}

		haircut, reason, ready := allocationHaircut(
			thesis,
			forecast,
			decimal.NewFromFloat64(0.1),
		)

		Convey("It should publish the exact penalty and its measured causes", func() {
			So(ready, ShouldBeTrue)
			So(haircut, ShouldAlmostEqual, 1.4/2.4, 1e-12)
			So(reason, ShouldEqual,
				"executable-depth scarcity + toxicity + adverse selection")
		})

		Convey("It should refuse to size without toxicity evidence", func() {
			thesis.Measurements.Delete(types.SourceToxicity)
			_, _, available := allocationHaircut(
				thesis,
				forecast,
				decimal.NewFromFloat64(0.1),
			)
			So(available, ShouldBeFalse)
		})
	})
}

func TestCandidateValuation(t *testing.T) {
	Convey("Given a candidate valuation", t, func() {
		c := candidate{
			Symbol:         "BTC/USD",
			ReferencePrice: decimal.NewFromFloat64(100.0),
			ExpectedReturn: decimal.NewFromFloat64(5.0),
			ExpectedFees:   decimal.NewFromFloat64(0.5),
			ExpectedSpread: decimal.NewFromFloat64(0.2),
			ExpectedImpact: decimal.NewFromFloat64(0.1),
		}

		Convey("ExecutableReturn should subtract all frictions", func() {
			netReturn := c.ExecutableReturn()
			So(netReturn.Float64(), ShouldAlmostEqual, 4.2)
		})

		Convey("FractionOf and RoundTripFraction should compute proportional friction correctly", func() {
			So(c.FractionOf(c.ExpectedFees), ShouldAlmostEqual, 0.005)
			So(c.RoundTripFraction(), ShouldAlmostEqual, 0.008)
			So(c.ExecutableFraction(), ShouldAlmostEqual, 0.042)
		})
	})
}

func BenchmarkUnifiedUtility(b *testing.B) {
	cognition := types.Cognition{
		Ready:          true,
		Confidence:     0.85,
		LookaheadPaths: 3,
	}
	causalTerm := causalFactor(0.2, 0.1, 0.05, true)
	cognitionTerm := cognitionFactor(cognition)
	graphTerm := graphFactor(10.0, 1.0, true)

	for b.Loop() {
		_ = unifiedUtility(0.025, causalTerm, cognitionTerm, graphTerm, 0.15)
	}
}

func BenchmarkProjectedReturn(b *testing.B) {
	curve := []float64{0.00024, 0.00025, 0.00026, 0.00027, 0.00028}
	surviving := []float64{0.9, 0.92, 0.95, 0.98, 1.0}

	for b.Loop() {
		_ = projectedReturn(curve, surviving)
	}
}

func BenchmarkAllocationHaircut(b *testing.B) {
	thesis := types.NewThesis()
	thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
		Source: types.SourceLiquidity,
		Symbol: "BTC/USD",
		Validity: types.MeasurementValidity{
			State: types.ValidityValid,
		},
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricScarcityScore, types.SideNone): {Raw: 0.5},
		},
	}})
	thesis.Measurements.Store(types.SourceToxicity, []*types.Measurement{{
		Source: types.SourceToxicity,
		Symbol: "BTC/USD",
		Validity: types.MeasurementValidity{
			State: types.ValidityValid,
		},
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricCancelledQuantity, types.SideBuy): {Raw: 4},
			types.MetricKey(types.MetricTouchQuantity, types.SideBuy):     {Raw: 6},
		},
	}})
	forecast := candidate{
		Symbol:         "BTC/USD",
		ExpectedSpread: decimal.NewFromFloat64(0.2),
	}
	adverse := decimal.NewFromFloat64(0.1)

	b.ResetTimer()

	for b.Loop() {
		_, _, _ = allocationHaircut(thesis, forecast, adverse)
	}
}
