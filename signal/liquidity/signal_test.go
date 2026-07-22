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
			{"spoof", tests.MarketStateSpoofLiquidity, []string{"SIM1/USD"},
				[]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"}},
			{"depth thinning", tests.MarketStateDepthThinning, []string{"SIM1/USD"}, nil},
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

			for _, measurement := range measurements {
				if measurement.Source != types.SourceLiquidity {
					continue
				}

				So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
				So(measurement.Validity.Readiness, ShouldEqual, types.ReadinessObservation)
				So(measurement.Subject, ShouldEqual, types.SubjectPeerLiquidity)
				So(measurement.Maturity, ShouldEqual, 3.0/4.0)
				So(measurement.Scale.Kind, ShouldEqual, types.ScaleObservationWindow)
				So(measurement.Scale.From, ShouldEqual, measurement.At)
				So(measurement.Scale.Through, ShouldEqual, measurement.At)

				switch measurement.Metric {
				case types.MetricRelativeTouchDepth, types.MetricScarcityScore:
					So(measurement.Unit, ShouldEqual, types.UnitDimensionless)

					if measurement.Raw == 0 {
						So(measurement.Normalized, ShouldBeNil)
						continue
					}

					So(measurement.Normalized, ShouldResemble, &measurement.Raw)
				default:
					So(measurement.Unit, ShouldEqual, types.UnitQuoteCurrency)
					So(measurement.Normalized, ShouldBeNil)
				}
			}

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
						value, found := check.values[metric][symbol]
						So(found, ShouldBeTrue)
						So(math.IsNaN(value), ShouldBeFalse)
						So(math.IsInf(value, 0), ShouldBeFalse)
					}

					if check.coherent {
						relative := check.values[types.MetricExecutableTouchDepth][symbol] /
							check.values[types.MetricExecutableTouchDepthMedian][symbol]
						So(check.values[types.MetricRelativeTouchDepth][symbol],
							ShouldEqual, relative)
						So(check.values[types.MetricScarcityScore][symbol],
							ShouldEqual, math.Max(0, 1-relative))
						So(check.values[types.MetricExecutableTouchDepthMedian][symbol],
							ShouldEqual, check.values[types.MetricExecutableTouchDepthMedian][check.symbols[0]])
						So(check.values[types.MetricReportedVolumeNotionalMedian][symbol],
							ShouldEqual, check.values[types.MetricReportedVolumeNotionalMedian][check.symbols[0]])
					}
				}
			}

			So(wired.Close(), ShouldBeNil)
			market.Close()
		}
		subject := "SIM1/USD"
		peers := []string{"SIM2/USD", "SIM3/USD"}

		for _, name := range []string{"loaded", "spoof"} {
			for _, metric := range metrics {
				for _, symbol := range []string{subject, peers[0], peers[1]} {
					So(outcomes[name].peak[metric][symbol], ShouldEqual,
						outcomes[name].calm[metric][symbol])
					So(outcomes[name].latest[metric][symbol], ShouldEqual,
						outcomes[name].calm[metric][symbol])
				}
			}
		}

		for _, metric := range metrics {
			So(outcomes["depth thinning"].peak[metric], ShouldHaveLength, 0)
			So(outcomes["depth thinning"].latest[metric], ShouldHaveLength, 0)
		}

		So(outcomes["retreat"].latest[types.MetricExecutableTouchDepth][subject],
			ShouldBeLessThan, outcomes["retreat"].calm[types.MetricExecutableTouchDepth][subject])
		So(outcomes["retreat"].latest[types.MetricRelativeTouchDepth][subject],
			ShouldBeLessThan, outcomes["retreat"].calm[types.MetricRelativeTouchDepth][subject])
		So(outcomes["retreat"].latest[types.MetricScarcityScore][subject],
			ShouldEqual, 0)

		So(outcomes["thin"].latest[types.MetricExecutableTouchDepth][subject],
			ShouldBeLessThan, outcomes["thin"].calm[types.MetricExecutableTouchDepth][subject])
		So(outcomes["thin"].latest[types.MetricRelativeTouchDepth][subject],
			ShouldBeLessThan, outcomes["thin"].calm[types.MetricRelativeTouchDepth][subject])
		So(outcomes["thin"].latest[types.MetricScarcityScore][subject],
			ShouldBeGreaterThan, outcomes["thin"].latest[types.MetricScarcityScore][peers[0]])

		for _, peer := range peers {
			So(outcomes["thin"].latest[types.MetricScarcityScore][peer], ShouldEqual, 0)
		}

		So(outcomes["isolated pump"].latest[types.MetricExecutableTouchDepth][subject],
			ShouldEqual, outcomes["cohort pump"].latest[types.MetricExecutableTouchDepth][subject])
		So(outcomes["isolated pump"].latest[types.MetricReportedVolumeNotional][subject],
			ShouldEqual, outcomes["cohort pump"].latest[types.MetricReportedVolumeNotional][subject])
		So(outcomes["isolated pump"].latest[types.MetricRelativeTouchDepth][subject],
			ShouldBeGreaterThan, outcomes["cohort pump"].latest[types.MetricRelativeTouchDepth][subject])
		So(outcomes["isolated pump"].latest[types.MetricReportedVolumeNotionalMedian][subject],
			ShouldBeLessThan, outcomes["cohort pump"].latest[types.MetricReportedVolumeNotionalMedian][subject])
	})
}
