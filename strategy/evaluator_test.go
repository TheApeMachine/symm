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

func TestMomentumMultiplier(t *testing.T) {
	Convey("Given an empty thesis", t, func() {
		thesis := types.NewThesis()

		Convey("It should return a neutral 1.0 multiplier", func() {
			So(momentumMultiplier(thesis, "BTC/USD"), ShouldEqual, 1.0)
		})
	})

	Convey("Given active ignition and Hawkes cascade evidence", t, func() {
		thesis := types.NewThesis()
		symbol := "SLAY/USD"

		pumpSample := types.MetricSample{Raw: 0.85}
		rvolSample := types.MetricSample{Raw: 12.0}
		trendSample := types.MetricSample{Raw: 0.75}

		pumpMeasurement := &types.Measurement{
			Source: types.SourcePumpDump,
			Symbol: symbol,
			Metrics: map[string]types.MetricSample{
				string(types.MetricKey(types.MetricIgnition, types.SideNone)): pumpSample,
				string(types.MetricKey(types.MetricRVOL, types.SideNone)):     rvolSample,
				string(types.MetricKey(types.MetricTrend, types.SideNone)):    trendSample,
			},
		}

		hawkesDescendants := types.MetricSample{Raw: 4.5}
		hawkesMeasurement := &types.Measurement{
			Source: types.SourceHawkes,
			Symbol: symbol,
			Metrics: map[string]types.MetricSample{
				string(types.MetricKey(types.MetricTotalDescendants, types.SideBuy)): hawkesDescendants,
			},
		}

		thesis.Measurements.Store(types.SourcePumpDump, []*types.Measurement{pumpMeasurement})
		thesis.Measurements.Store(types.SourceHawkes, []*types.Measurement{hawkesMeasurement})

		var multiplier float64

		Convey("It should dynamically scale the momentum multiplier across volume and cascade horizons", func() {
			multiplier = momentumMultiplier(thesis, symbol)

			So(multiplier, ShouldBeGreaterThan, 10.0)
		})

		Convey("Given active breakout categories", func() {
			baseMultiplier := momentumMultiplier(thesis, symbol)
			thesis.Categories[symbol] = []types.Category{
				{
					Type:       types.CategoryVerticalIgnition,
					Strength:   0.9,
					Confidence: 0.85,
					Surprisal:  0.1,
				},
			}

			Convey("It should amplify the opportunity multiplier further", func() {
				boosted := momentumMultiplier(thesis, symbol)

				So(boosted, ShouldBeGreaterThan, baseMultiplier)
			})
		})

		Convey("Given market exhaustion signals", func() {
			thesis.Categories[symbol] = []types.Category{
				{
					Type:       types.CategoryExhaustion,
					Strength:   1.0,
					Confidence: 1.0,
				},
			}

			Convey("It should collapse the multiplier to zero to prevent buying into a dump", func() {
				dampened := momentumMultiplier(thesis, symbol)

				So(dampened, ShouldEqual, 0.0)
			})
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

func BenchmarkMomentumMultiplier(b *testing.B) {
	thesis := types.NewThesis()
	symbol := "SLAY/USD"

	pumpMeasurement := &types.Measurement{
		Source: types.SourcePumpDump,
		Symbol: symbol,
		Metrics: map[string]types.MetricSample{
			string(types.MetricKey(types.MetricIgnition, types.SideNone)): {Raw: 0.85},
			string(types.MetricKey(types.MetricRVOL, types.SideNone)):     {Raw: 10.0},
			string(types.MetricKey(types.MetricTrend, types.SideNone)):    {Raw: 0.5},
		},
	}

	hawkesMeasurement := &types.Measurement{
		Source: types.SourceHawkes,
		Symbol: symbol,
		Metrics: map[string]types.MetricSample{
			string(types.MetricKey(types.MetricTotalDescendants, types.SideBuy)): {Raw: 3.5},
		},
	}

	thesis.Measurements.Store(types.SourcePumpDump, []*types.Measurement{pumpMeasurement})
	thesis.Measurements.Store(types.SourceHawkes, []*types.Measurement{hawkesMeasurement})
	thesis.Categories[symbol] = []types.Category{
		{
			Type:       types.CategoryVerticalIgnition,
			Strength:   0.8,
			Confidence: 0.8,
			Surprisal:  0.2,
		},
	}

	for b.Loop() {
		_ = momentumMultiplier(thesis, symbol)
	}
}
