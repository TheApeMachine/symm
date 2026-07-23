package depthflow_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

type metricValues = map[types.MetricType]map[string]float64

/*
marketOutcome retains the strongest and final depth classification from a tape.
*/
type marketOutcome struct {
	peak   metricValues
	latest metricValues
}

/*
TestCalculate proves depthflow distinguishes loaded, spoofed, thinned, and
neutral books without treating ordinary price direction as book geometry.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricLoadedScore,
		types.MetricSpoofScore,
		types.MetricThinScore,
		types.MetricNeutralScore,
		types.MetricStrength,
		types.MetricValue,
	}
	families := metrics[:4]

	Convey("Given causal and adversarial book-shape tapes", t, func() {
		proofs := []struct {
			name   string
			state  tests.MarketState
			family types.MetricType
		}{
			{"baseline", tests.MarketStateBaseline, types.MetricNeutralScore},
			{"directional", tests.MarketStateFastPump, types.MetricNeutralScore},
			{"compression", tests.MarketStateSpreadCompression, types.MetricNeutralScore},
			{"thin touch", tests.MarketStateThinLiquidity, types.MetricLoadedScore},
			{"retreat", tests.MarketStateLiquidityRetreat, types.MetricLoadedScore},
			{"loaded", tests.MarketStateLoadedLiquidity, types.MetricLoadedScore},
			{"spoof", tests.MarketStateSpoofLiquidity, types.MetricSpoofScore},
			{"thinning", tests.MarketStateDepthThinning, types.MetricThinScore},
		}
		outcomes := make(map[string]marketOutcome, len(proofs))

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), 3)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(tests.Idle), ShouldBeNil)
			measurements := []*types.Measurement{}
			So(market.Transition(proof.state, func() error {
				thesis := wired.Thesis

				measurements = append(measurements, thesis.Measurements...)
				return nil
			}), ShouldBeNil)

			for _, measurement := range measurements {
				if measurement.Source != types.SourceDepthFlow {
					continue
				}

				So(measurement.ValidateStruct(), ShouldBeNil)
				So(measurement.Stream, ShouldEqual, types.DepthFlow)
				So(measurement.Subject, ShouldEqual, types.SubjectBookImbalance)
				So(measurement.Unit, ShouldEqual, types.UnitDimensionless)
				So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
				So(measurement.Validity.Readiness, ShouldEqual, types.ReadinessObservation)

				if measurement.Raw == 0 {
					So(measurement.Normalized, ShouldBeNil)
					continue
				}

				So(measurement.Normalized, ShouldNotBeNil)
				So(*measurement.Normalized, ShouldEqual, measurement.Raw)
			}

			outcomes[proof.name] = marketOutcome{
				peak: tests.PeakMeasurements(measurements, types.SourceDepthFlow, metrics),
				latest: tests.LatestMeasurements(
					measurements, types.SourceDepthFlow, metrics,
				),
			}
			for _, values := range []metricValues{
				outcomes[proof.name].peak, outcomes[proof.name].latest,
			} {
				for _, metric := range metrics {
					So(values[metric], ShouldHaveLength, 3)
				}

				for _, symbol := range market.Symbols {
					for _, metric := range metrics {
						So(math.IsNaN(values[metric][symbol]), ShouldBeFalse)
						So(math.IsInf(values[metric][symbol], 0), ShouldBeFalse)
					}

					for _, metric := range families {
						So(values[metric][symbol], ShouldBeGreaterThanOrEqualTo, 0)
					}

					strength := 0.0

					for _, metric := range families {
						strength = max(strength, values[metric][symbol])
					}

					So(values[types.MetricStrength][symbol], ShouldEqual, strength)
					So(values[types.MetricStrength][symbol], ShouldBeGreaterThan, 0)
					So(values[types.MetricValue][symbol], ShouldEqual,
						values[types.MetricStrength][symbol])
				}
			}

			for _, symbol := range market.Symbols {
				claim := proof.name + " " + symbol + " " + string(proof.family)
				SoMsg(claim+" peak", outcomes[proof.name].peak[proof.family][symbol],
					ShouldBeGreaterThan, 0)
				SoMsg(claim+" latest", outcomes[proof.name].latest[proof.family][symbol],
					ShouldBeGreaterThan, 0)

				for _, family := range families {
					if family == proof.family {
						continue
					}

					So(outcomes[proof.name].latest[family][symbol], ShouldEqual, 0)
				}
			}

			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		compressionLoadedScore := 0.0
		loadedScore := 0.0
		directionalLoadedScore := 0.0

		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			loadedScore += outcomes["loaded"].latest[types.MetricLoadedScore][symbol]
			compressionLoadedScore +=
				outcomes["compression"].latest[types.MetricLoadedScore][symbol]
			directionalLoadedScore +=
				outcomes["directional"].latest[types.MetricLoadedScore][symbol]
		}

		So(loadedScore, ShouldBeGreaterThan, directionalLoadedScore)
		So(loadedScore, ShouldBeGreaterThan, compressionLoadedScore)
	})
}
