package depthflow_test

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
metricValues retains one exact score per metric and emitted symbol.
*/
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
			name         string
			state        tests.MarketState
			family       types.MetricType
			peak         float64
			latest       float64
			measurements int
			subjectOnly  bool
		}{
			{"baseline", tests.MarketStateBaseline, types.MetricNeutralScore,
				1, 1, 576, false},
			{"directional", tests.MarketStateFastPump, types.MetricNeutralScore,
				1, 1, 432, false},
			{"compression", tests.MarketStateSpreadCompression, types.MetricNeutralScore,
				1, 1, 432, false},
			{"spread control", tests.MarketStateSpreadControl, types.MetricNeutralScore,
				1, 1, 432, false},
			{"thin touch", tests.MarketStateThinLiquidity, types.MetricLoadedScore,
				0.64013471225271312, 0.64013471225271312, 6, true},
			{"retreat", tests.MarketStateLiquidityRetreat, types.MetricLoadedScore,
				0.46353314655395195, 0.46353314655395195, 6, true},
			{"loaded", tests.MarketStateLoadedLiquidity, types.MetricLoadedScore,
				0.53020087811712591, 0.0075421382831116828, 108, true},
			{"spoof", tests.MarketStateSpoofLiquidity, types.MetricSpoofScore,
				1.0893181558840284, 1.0893181558840284, 6, true},
			{"thinning", tests.MarketStateDepthThinning, types.MetricThinScore,
				0.43451479078700339, 0.43451479078700339, 6, true},
		}

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), 3)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(tests.Consume(wired.Crypto.Tick)), ShouldBeNil)
			measurements := []*types.Measurement{}
			So(market.Transition(proof.state, func() error {
				thesis, err := wired.Crypto.Tick()

				if err != nil {
					return err
				}

				for _, measurement := range thesis.Measurements {
					if measurement.Source == types.SourceDepthFlow {
						measurements = append(measurements, measurement)
					}
				}

				return nil
			}, market.Symbols[0]), ShouldBeNil)
			So(measurements, ShouldHaveLength, proof.measurements)

			for _, measurement := range measurements {
				So(measurement.ValidateStruct(), ShouldBeNil)
				So(measurement.Source, ShouldEqual, types.SourceDepthFlow)
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

			outcome := marketOutcome{
				peak: utils.PeakMeasurements(measurements, types.SourceDepthFlow, metrics),
				latest: utils.LatestMeasurements(
					measurements, types.SourceDepthFlow, metrics,
				),
			}
			expectedSymbols := market.Symbols

			if proof.subjectOnly {
				expectedSymbols = market.Symbols[:1]
			}

			for _, check := range []struct {
				name   string
				values metricValues
				score  float64
			}{
				{"peak", outcome.peak, proof.peak},
				{"latest", outcome.latest, proof.latest},
			} {
				for _, metric := range metrics {
					SoMsg(proof.name+" "+check.name+" symbols",
						check.values[metric], ShouldHaveLength, len(expectedSymbols))

					for _, symbol := range expectedSymbols {
						So(check.values[metric], ShouldContainKey, symbol)
					}
				}

				for _, symbol := range expectedSymbols {
					for _, metric := range metrics {
						So(math.IsNaN(check.values[metric][symbol]), ShouldBeFalse)
						So(math.IsInf(check.values[metric][symbol], 0), ShouldBeFalse)
					}

					expectedFamily := proof.family
					expectedScore := check.score

					if symbol != market.Symbols[0] {
						expectedFamily = types.MetricNeutralScore
						expectedScore = 1
					}

					familyMass := 0.0

					for _, metric := range families {
						expected := 0.0

						if metric == expectedFamily {
							expected = expectedScore
						}

						SoMsg(proof.name+" "+check.name+" "+symbol+" "+string(metric),
							check.values[metric][symbol], ShouldEqual, expected)
						familyMass += check.values[metric][symbol]
					}

					So(check.values[types.MetricStrength][symbol], ShouldEqual, familyMass)
					So(check.values[types.MetricValue][symbol], ShouldEqual, familyMass)
				}
			}

			So(wired.Close(), ShouldBeNil)
			market.Close()
		}
	})
}
