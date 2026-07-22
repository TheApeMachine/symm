package liquidity_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
metricValues indexes each liquidity quantity by metric and market symbol.
*/
type metricValues = map[types.MetricType]map[string]float64

/*
marketOutcome retains transient and final cross-sectional liquidity evidence.
*/
type marketOutcome struct {
	calm   metricValues
	peak   metricValues
	latest metricValues
}

/*
TestCalculate proves liquidity distinguishes ordinary, loaded, depleted,
retreating, and isolated touches through the production boot graph.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricExecutableTouchDepth,
		types.MetricRelativeTouchDepth,
		types.MetricScarcityScore,
		types.MetricExecutableTouchDepthMedian,
		types.MetricReportedVolumeNotional,
		types.MetricReportedVolumeNotionalMedian,
	}

	Convey("Given ordinary and stressed cross-sectional liquidity tapes", t, func() {
		proofs := []struct {
			name     string
			state    tests.MarketState
			symbols  []string
			expected []string
		}{
			{"baseline", tests.MarketStateBaseline, nil,
				[]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"}},
			{"cohort pump", tests.MarketStateFastPump, nil,
				[]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"}},
			{"thin", tests.MarketStateThinLiquidity, []string{"SIM1/USD"},
				[]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"}},
			{"loaded", tests.MarketStateLoadedLiquidity, []string{"SIM1/USD"},
				[]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"}},
			{"retreat", tests.MarketStateLiquidityRetreat, []string{"SIM1/USD"},
				[]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"}},
			{"isolated pump", tests.MarketStateFastPump, []string{"SIM1/USD"},
				[]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"}},
		}
		outcomes := make(map[string]marketOutcome, len(proofs))

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), 3)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			var calmThesis *types.Thesis
			So(market.Warmup(func() error {
				thesis, err := wired.Crypto.Tick()

				if err != nil {
					return err
				}

				calmThesis = thesis
				return nil
			}), ShouldBeNil)
			calm := utils.LatestMeasurements(
				calmThesis.Measurements, types.SourceLiquidity, metrics,
			)
			measurements := []*types.Measurement{}
			So(market.Transition(proof.state, func() error {
				thesis, err := wired.Crypto.Tick()

				if err != nil {
					return err
				}

				measurements = append(measurements, thesis.Measurements...)
				return nil
			}, proof.symbols...), ShouldBeNil)
			outcomes[proof.name] = marketOutcome{
				calm:   calm,
				peak:   utils.PeakMeasurements(measurements, types.SourceLiquidity, metrics),
				latest: utils.LatestMeasurements(measurements, types.SourceLiquidity, metrics),
			}

			checks := []struct {
				values   metricValues
				symbols  []string
				coherent bool
			}{
				{outcomes[proof.name].calm, market.Symbols, true},
				{outcomes[proof.name].peak, proof.expected, false},
				{outcomes[proof.name].latest, proof.expected, true},
			}

			for _, check := range checks {
				for _, metric := range metrics {
					So(check.values[metric], ShouldHaveLength, len(check.symbols))
				}

				for _, symbol := range check.symbols {
					for _, metric := range metrics {
						So(math.IsNaN(check.values[metric][symbol]), ShouldBeFalse)
						So(math.IsInf(check.values[metric][symbol], 0), ShouldBeFalse)
						So(check.values[metric][symbol], ShouldBeGreaterThanOrEqualTo, 0)
					}

					So(check.values[types.MetricExecutableTouchDepth][symbol], ShouldBeGreaterThan, 0)
					So(check.values[types.MetricRelativeTouchDepth][symbol], ShouldBeGreaterThan, 0)
					So(check.values[types.MetricExecutableTouchDepthMedian][symbol], ShouldBeGreaterThan, 0)
					So(check.values[types.MetricReportedVolumeNotional][symbol], ShouldBeGreaterThan, 0)
					So(check.values[types.MetricReportedVolumeNotionalMedian][symbol], ShouldBeGreaterThan, 0)

					if check.coherent {
						relative := check.values[types.MetricExecutableTouchDepth][symbol] /
							check.values[types.MetricExecutableTouchDepthMedian][symbol]
						So(check.values[types.MetricRelativeTouchDepth][symbol],
							ShouldAlmostEqual, relative)
						So(check.values[types.MetricScarcityScore][symbol],
							ShouldAlmostEqual, math.Max(0, 1-relative))
					}
				}
			}

			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		subject := "SIM1/USD"

		for _, metric := range metrics {
			So(outcomes["loaded"].latest[metric][subject], ShouldAlmostEqual,
				outcomes["loaded"].calm[metric][subject])
		}

		So(outcomes["retreat"].latest[types.MetricExecutableTouchDepth][subject],
			ShouldBeLessThan, outcomes["retreat"].calm[types.MetricExecutableTouchDepth][subject])
		So(outcomes["retreat"].latest[types.MetricRelativeTouchDepth][subject],
			ShouldBeLessThan, outcomes["retreat"].calm[types.MetricRelativeTouchDepth][subject])
		So(outcomes["retreat"].latest[types.MetricScarcityScore][subject],
			ShouldEqual, outcomes["retreat"].calm[types.MetricScarcityScore][subject])

		So(outcomes["thin"].latest[types.MetricExecutableTouchDepth][subject],
			ShouldBeLessThan, outcomes["thin"].calm[types.MetricExecutableTouchDepth][subject])
		So(outcomes["thin"].latest[types.MetricRelativeTouchDepth][subject],
			ShouldBeLessThan, outcomes["thin"].calm[types.MetricRelativeTouchDepth][subject])
		So(outcomes["thin"].latest[types.MetricScarcityScore][subject],
			ShouldBeGreaterThan, outcomes["thin"].calm[types.MetricScarcityScore][subject])

		So(outcomes["isolated pump"].latest[types.MetricExecutableTouchDepth][subject],
			ShouldAlmostEqual, outcomes["cohort pump"].latest[types.MetricExecutableTouchDepth][subject])
		So(outcomes["isolated pump"].latest[types.MetricReportedVolumeNotional][subject],
			ShouldAlmostEqual, outcomes["cohort pump"].latest[types.MetricReportedVolumeNotional][subject])
		So(outcomes["isolated pump"].latest[types.MetricRelativeTouchDepth][subject],
			ShouldBeGreaterThan, outcomes["cohort pump"].latest[types.MetricRelativeTouchDepth][subject])
		So(outcomes["isolated pump"].latest[types.MetricReportedVolumeNotionalMedian][subject],
			ShouldBeLessThan, outcomes["cohort pump"].latest[types.MetricReportedVolumeNotionalMedian][subject])
	})
}
