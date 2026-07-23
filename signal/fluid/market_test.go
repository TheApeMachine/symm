package fluid_test

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
metricValues groups every requested fluid metric by simulated symbol.
*/
type metricValues = map[types.MetricType]map[string]float64

/*
marketOutcome retains the strongest and settled fluid state from one tape.
*/
type marketOutcome struct {
	peak   metricValues
	latest metricValues
}

/*
TestCalculate proves every Fluid score and physical reading across directional,
compressed, depleted, loaded, rejecting, and reversing production tapes.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricLaminarScore,
		types.MetricTurbulentScore,
		types.MetricInertialScore,
		types.MetricViscousScore,
		types.MetricViscosity,
		types.MetricReynolds,
		types.MetricDivergenceV2,
		types.MetricVelocityCurvatureV2,
		types.MetricTurbulence,
		types.MetricSourceBalance,
		types.MetricMemory,
		types.MetricMidAddRate,
		types.MetricMidExecuteRate,
	}
	families := metrics[:4]

	Convey("Given directional and structural market tapes", t, func() {
		proofs := []struct {
			name   string
			states []tests.MarketState
		}{
			{"baseline", []tests.MarketState{tests.MarketStateBaseline}},
			{"fast pump", []tests.MarketState{tests.MarketStateFastPump}},
			{"slow pump", []tests.MarketState{tests.MarketStateSlowPump}},
			{"fast dump", []tests.MarketState{tests.MarketStateFastDump}},
			{"slow dump", []tests.MarketState{tests.MarketStateSlowDump}},
			{"absorption", []tests.MarketState{tests.MarketStateVolumeAbsorption}},
			{"low-volume lift", []tests.MarketState{tests.MarketStateLowVolumeLift}},
			{"compression", []tests.MarketState{tests.MarketStateSpreadCompression}},
			{"thin", []tests.MarketState{tests.MarketStateThinLiquidity}},
			{"loaded", []tests.MarketState{tests.MarketStateLoadedLiquidity}},
			{"retreat", []tests.MarketState{tests.MarketStateLiquidityRetreat}},
			{"fast rejection", []tests.MarketState{
				tests.MarketStateFastPump, tests.MarketStateFastDump,
			}},
			{"reversal", []tests.MarketState{
				tests.MarketStateSlowDump, tests.MarketStateFastPump,
			}},
		}
		outcomes := make(map[string]marketOutcome, len(proofs))

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), 3)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(tests.Consume(wired.Crypto.Tick)), ShouldBeNil)
			measurements := []*types.Measurement{}
			var latest []*types.Measurement

			for index, state := range proof.states {
				capture := index == len(proof.states)-1
				So(market.Transition(state, func() error {
					thesis, err := wired.Crypto.Tick()

					if err != nil {
						return err
					}

					if capture {
						measurements = append(measurements, thesis.Measurements...)
						latest = thesis.Measurements
					}

					return nil
				}), ShouldBeNil)
			}

			outcomes[proof.name] = marketOutcome{
				peak: utils.PeakMeasurements(measurements, types.SourceFluid, metrics),
				latest: utils.LatestMeasurements(
					latest, types.SourceFluid, metrics,
				),
			}

			for metric, values := range utils.PeakMagnitudeMeasurements(
				measurements,
				types.SourceFluid,
				[]types.MetricType{
					types.MetricDivergenceV2,
					types.MetricSourceBalance,
				},
			) {
				outcomes[proof.name].peak[metric] = values
			}
			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		for _, outcome := range outcomes {
			for _, values := range []metricValues{outcome.peak, outcome.latest} {
				for _, metric := range metrics {
					So(values[metric], ShouldHaveLength, 3)
				}

				for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
					familyPeak := 0.0

					for _, metric := range metrics {
						So(math.IsNaN(values[metric][symbol]), ShouldBeFalse)
						So(math.IsInf(values[metric][symbol], 0), ShouldBeFalse)
					}

					for _, metric := range families {
						So(values[metric][symbol], ShouldBeGreaterThanOrEqualTo, 0)
						familyPeak = max(familyPeak, values[metric][symbol])
					}

					So(familyPeak, ShouldBeGreaterThan, 0)
					So(values[types.MetricViscosity][symbol], ShouldBeGreaterThan, 0)
					So(values[types.MetricReynolds][symbol], ShouldBeGreaterThanOrEqualTo, 0)
					So(values[types.MetricMemory][symbol], ShouldBeBetweenOrEqual, 0, 1)
					So(values[types.MetricMidAddRate][symbol], ShouldBeGreaterThanOrEqualTo, 0)
					So(values[types.MetricMidExecuteRate][symbol], ShouldBeGreaterThanOrEqualTo, 0)
				}
			}
		}
		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			for _, comparison := range []struct {
				stronger string
				weaker   string
				metrics  []types.MetricType
			}{
				{"fast pump", "slow pump", []types.MetricType{
					types.MetricMidAddRate, types.MetricMidExecuteRate, types.MetricReynolds,
					types.MetricVelocityCurvatureV2, types.MetricTurbulence,
					types.MetricInertialScore,
				}},
				{"fast dump", "slow dump", []types.MetricType{
					types.MetricMidAddRate, types.MetricMidExecuteRate, types.MetricReynolds,
					types.MetricVelocityCurvatureV2, types.MetricTurbulence,
				}},
				{"fast pump", "absorption", []types.MetricType{
					types.MetricVelocityCurvatureV2, types.MetricTurbulence,
				}},
				{"thin", "loaded", []types.MetricType{
					types.MetricTurbulentScore, types.MetricReynolds,
					types.MetricVelocityCurvatureV2, types.MetricTurbulence,
				}},
				{"fast rejection", "baseline", []types.MetricType{
					types.MetricMidAddRate, types.MetricMidExecuteRate, types.MetricReynolds,
					types.MetricVelocityCurvatureV2, types.MetricTurbulence,
				}},
				{"reversal", "baseline", []types.MetricType{
					types.MetricMidAddRate, types.MetricMidExecuteRate, types.MetricReynolds,
					types.MetricVelocityCurvatureV2, types.MetricTurbulence,
				}},
				{"fast pump", "low-volume lift", []types.MetricType{
					types.MetricMidExecuteRate,
				}},
			} {
				for _, metric := range comparison.metrics {
					So(outcomes[comparison.stronger].peak[metric][symbol], ShouldBeGreaterThan,
						outcomes[comparison.weaker].peak[metric][symbol])
				}
			}

			for _, comparison := range []struct {
				stronger string
				weaker   string
				metrics  []types.MetricType
			}{
				{"fast pump", "absorption", []types.MetricType{types.MetricMemory}},
			} {
				for _, metric := range comparison.metrics {
					So(outcomes[comparison.stronger].latest[metric][symbol], ShouldBeGreaterThan,
						outcomes[comparison.weaker].latest[metric][symbol])
				}
			}

			for _, name := range []string{
				"baseline", "fast pump", "slow pump", "fast dump", "slow dump",
				"low-volume lift", "compression", "fast rejection", "reversal",
			} {
				So(outcomes[name].peak[types.MetricMidAddRate][symbol], ShouldBeGreaterThan, 0)
				So(outcomes[name].peak[types.MetricMidExecuteRate][symbol], ShouldBeGreaterThan, 0)
			}

			for _, name := range []string{
				"fast pump", "slow pump", "fast dump", "slow dump",
				"low-volume lift", "fast rejection", "reversal",
			} {
				So(outcomes[name].peak[types.MetricVelocityCurvatureV2][symbol], ShouldBeGreaterThan, 0)
				So(outcomes[name].peak[types.MetricTurbulence][symbol], ShouldBeGreaterThan, 0)
				So(math.Abs(outcomes[name].peak[types.MetricDivergenceV2][symbol]), ShouldBeGreaterThan, 0)
			}

			for _, expectation := range []struct {
				name     string
				balanced bool
				add      bool
				execute  bool
			}{
				{"absorption", true, true, true},
				{"loaded", false, true, false},
				{"thin", false, true, false},
				{"retreat", false, false, false},
			} {
				peak := outcomes[expectation.name].peak

				if expectation.balanced {
					SoMsg(expectation.name+" "+symbol+" source balance",
						peak[types.MetricSourceBalance][symbol], ShouldEqual, 0)
				}

				if !expectation.balanced && expectation.name != "retreat" {
					SoMsg(expectation.name+" "+symbol+" source balance",
						math.Abs(peak[types.MetricSourceBalance][symbol]), ShouldBeGreaterThan, 0)
				}

				if expectation.name == "retreat" {
					SoMsg(expectation.name+" "+symbol+" source balance",
						peak[types.MetricSourceBalance][symbol], ShouldBeLessThanOrEqualTo, 0)
				}

				if expectation.name == "absorption" || expectation.name == "loaded" {
					So(peak[types.MetricMidAddRate][symbol] > 0, ShouldEqual, expectation.add)
				}

				So(peak[types.MetricMidExecuteRate][symbol] > 0, ShouldEqual, expectation.execute)

				if expectation.name == "thin" || expectation.name == "retreat" {
					So(peak[types.MetricTurbulence][symbol], ShouldBeGreaterThan, 0)
				}
			}

			So(outcomes["absorption"].peak[types.MetricMidExecuteRate][symbol],
				ShouldAlmostEqual, outcomes["fast pump"].peak[types.MetricMidExecuteRate][symbol])
			So(outcomes["retreat"].peak[types.MetricViscosity][symbol], ShouldBeLessThan,
				outcomes["baseline"].peak[types.MetricViscosity][symbol])

			for _, metric := range []types.MetricType{
				types.MetricMemory, types.MetricViscosity,
			} {
				So(outcomes["low-volume lift"].latest[metric][symbol], ShouldAlmostEqual,
					outcomes["fast pump"].latest[metric][symbol])
			}
		}

		retreatNegative := false

		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			retreatNegative = retreatNegative ||
				outcomes["retreat"].peak[types.MetricSourceBalance][symbol] < 0
		}

		So(retreatNegative, ShouldBeTrue)
	})
}
