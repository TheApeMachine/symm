package pumpdump_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

type metricValues = map[types.MetricType]map[string]float64

const (
	baselineStep = -1
	currentStep  = -2
	firstStep    = 0
)

/*
metricReference identifies the snapshot and metric supplying an expectation.
*/
type metricReference struct {
	step   int
	metric types.MetricType
}

/*
metricMaximum expresses the production composite contract without assuming
which evidence family dominates a particular market step.
*/
type metricMaximum []types.MetricType

/*
metricExpectation pairs one GoConvey assertion with a literal or metric reference.
*/
type metricExpectation struct {
	metric   types.MetricType
	assert   Assertion
	expected any
}

/*
TestMeasure proves every pump transition against the pumpdump measurements
produced through the complete production boot graph.
*/
func TestMeasure(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricRVOL, types.MetricPrecursor, types.MetricSpread,
		types.MetricCompression, types.MetricIgnition, types.MetricTrend,
		types.MetricExhaustion, types.MetricStrength,
	}
	baseline := []metricExpectation{
		{types.MetricRVOL, ShouldBeGreaterThan, 0.0},
		{types.MetricPrecursor, ShouldBeGreaterThanOrEqualTo, 0.0},
		{types.MetricSpread, ShouldBeGreaterThan, 0.0},
		{types.MetricCompression, ShouldEqual, 0.0},
		{types.MetricIgnition, ShouldBeGreaterThanOrEqualTo, 0.0},
		{types.MetricTrend, ShouldBeGreaterThanOrEqualTo, 0.0},
		{types.MetricExhaustion, ShouldBeGreaterThanOrEqualTo, 0.0},
		{types.MetricStrength, ShouldEqual, metricMaximum{
			types.MetricCompression, types.MetricIgnition,
			types.MetricTrend, types.MetricExhaustion,
		}},
	}
	cases := []struct {
		name   string
		states []tests.MarketState
		peaks  []metricExpectation
		latest []metricExpectation
	}{
		{
			name:   "When a fast pump ignites",
			states: []tests.MarketState{tests.MarketStateFastPump},
			peaks: []metricExpectation{
				{types.MetricRVOL, ShouldBeGreaterThan, metricReference{baselineStep, types.MetricRVOL}},
				{types.MetricPrecursor, ShouldBeGreaterThan, metricReference{baselineStep, types.MetricPrecursor}},
				{types.MetricSpread, ShouldAlmostEqual, metricReference{baselineStep, types.MetricSpread}},
				{types.MetricCompression, ShouldEqual, 0.0},
				{types.MetricIgnition, ShouldBeGreaterThan, metricReference{baselineStep, types.MetricIgnition}},
				{types.MetricTrend, ShouldBeGreaterThan, metricReference{baselineStep, types.MetricTrend}},
				{types.MetricExhaustion, ShouldBeGreaterThanOrEqualTo, 0.0},
				{types.MetricStrength, ShouldEqual, metricMaximum{
					types.MetricCompression, types.MetricIgnition,
					types.MetricTrend, types.MetricExhaustion,
				}},
			},
		},
		{
			name: "When a fast pump becomes a fast dump",
			states: []tests.MarketState{
				tests.MarketStateFastPump, tests.MarketStateFastDump,
			},
			peaks: []metricExpectation{
				{types.MetricRVOL, ShouldBeGreaterThan, 0.0},
				{types.MetricPrecursor, ShouldBeLessThan, metricReference{firstStep, types.MetricPrecursor}},
				{types.MetricSpread, ShouldAlmostEqual, metricReference{firstStep, types.MetricSpread}},
				{types.MetricCompression, ShouldEqual, 0.0},
				{types.MetricIgnition, ShouldBeLessThan, metricReference{firstStep, types.MetricIgnition}},
				{types.MetricTrend, ShouldBeLessThan, metricReference{firstStep, types.MetricTrend}},
				{types.MetricExhaustion, ShouldBeGreaterThan, 0.0},
				{types.MetricStrength, ShouldEqual, metricMaximum{
					types.MetricCompression, types.MetricIgnition,
					types.MetricTrend, types.MetricExhaustion,
				}},
			},
			latest: []metricExpectation{
				{types.MetricRVOL, ShouldBeLessThan, metricReference{firstStep, types.MetricRVOL}},
				{types.MetricStrength, ShouldEqual, metricMaximum{
					types.MetricCompression, types.MetricIgnition,
					types.MetricTrend, types.MetricExhaustion,
				}},
			},
		},
		{
			name: "When a fast pump exhausts into a slow dump",
			states: []tests.MarketState{
				tests.MarketStateFastPump, tests.MarketStateSlowDump,
			},
			peaks: []metricExpectation{
				{types.MetricRVOL, ShouldBeLessThan, metricReference{firstStep, types.MetricRVOL}},
				{types.MetricPrecursor, ShouldBeLessThan, metricReference{firstStep, types.MetricPrecursor}},
				{types.MetricSpread, ShouldAlmostEqual, metricReference{firstStep, types.MetricSpread}},
				{types.MetricCompression, ShouldEqual, 0.0},
				{types.MetricIgnition, ShouldBeLessThan, metricReference{firstStep, types.MetricIgnition}},
				{types.MetricTrend, ShouldBeLessThan, metricReference{firstStep, types.MetricTrend}},
				{types.MetricExhaustion, ShouldBeGreaterThan, 0.0},
				{types.MetricStrength, ShouldEqual, metricMaximum{
					types.MetricCompression, types.MetricIgnition,
					types.MetricTrend, types.MetricExhaustion,
				}},
				{types.MetricStrength, ShouldBeLessThan, metricReference{firstStep, types.MetricStrength}},
			},
			latest: []metricExpectation{
				{types.MetricStrength, ShouldEqual, metricMaximum{
					types.MetricCompression, types.MetricIgnition,
					types.MetricTrend, types.MetricExhaustion,
				}},
			},
		},
		{
			name:   "When a slow pump sustains ignition",
			states: []tests.MarketState{tests.MarketStateSlowPump},
			peaks: []metricExpectation{
				{types.MetricRVOL, ShouldBeGreaterThan, 0.0},
				{types.MetricPrecursor, ShouldBeGreaterThan, metricReference{baselineStep, types.MetricPrecursor}},
				{types.MetricSpread, ShouldAlmostEqual, metricReference{baselineStep, types.MetricSpread}},
				{types.MetricCompression, ShouldEqual, 0.0},
				{types.MetricIgnition, ShouldBeGreaterThan, metricReference{baselineStep, types.MetricIgnition}},
				{types.MetricTrend, ShouldBeGreaterThan, metricReference{baselineStep, types.MetricTrend}},
				{types.MetricExhaustion, ShouldBeGreaterThanOrEqualTo, 0.0},
				{types.MetricStrength, ShouldEqual, metricMaximum{
					types.MetricCompression, types.MetricIgnition,
					types.MetricTrend, types.MetricExhaustion,
				}},
			},
		},
		{
			name:   "When a slow dump rejects without ignition",
			states: []tests.MarketState{tests.MarketStateSlowDump},
			peaks: []metricExpectation{
				{types.MetricRVOL, ShouldBeGreaterThan, 0.0},
				{types.MetricPrecursor, ShouldEqual, 0.0},
				{types.MetricSpread, ShouldAlmostEqual, metricReference{baselineStep, types.MetricSpread}},
				{types.MetricCompression, ShouldEqual, 0.0},
				{types.MetricIgnition, ShouldEqual, 0.0},
				{types.MetricTrend, ShouldEqual, 0.0},
				{types.MetricExhaustion, ShouldBeGreaterThan, 0.0},
				{types.MetricStrength, ShouldEqual, metricMaximum{
					types.MetricCompression, types.MetricIgnition,
					types.MetricTrend, types.MetricExhaustion,
				}},
			},
			latest: []metricExpectation{
				{types.MetricStrength, ShouldEqual, metricMaximum{
					types.MetricCompression, types.MetricIgnition,
					types.MetricTrend, types.MetricExhaustion,
				}},
			},
		},
		{
			name: "When a slow dump reverses into a fast pump",
			states: []tests.MarketState{
				tests.MarketStateSlowDump, tests.MarketStateFastPump,
			},
			peaks: []metricExpectation{
				{types.MetricRVOL, ShouldBeGreaterThan, metricReference{firstStep, types.MetricRVOL}},
				{types.MetricPrecursor, ShouldEqual, 0.0},
				{types.MetricSpread, ShouldAlmostEqual, metricReference{firstStep, types.MetricSpread}},
				{types.MetricCompression, ShouldEqual, 0.0},
				{types.MetricIgnition, ShouldEqual, 0.0},
				{types.MetricTrend, ShouldEqual, 0.0},
				{types.MetricExhaustion, ShouldBeGreaterThan, 0.0},
				{types.MetricStrength, ShouldEqual, metricMaximum{
					types.MetricCompression, types.MetricIgnition,
					types.MetricTrend, types.MetricExhaustion,
				}},
			},
			latest: []metricExpectation{
				{types.MetricStrength, ShouldEqual, metricMaximum{
					types.MetricCompression, types.MetricIgnition,
					types.MetricTrend, types.MetricExhaustion,
				}},
			},
		},
	}

	Convey("Given a baseline market", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		So(market.Bootstrap(), ShouldBeNil)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() { wired.Close(); market.Close() })

		So(market.Warmup(wired.Crypto.Tick), ShouldBeNil)
		calm := utils.LatestMeasurements(
			wired.Crypto.Thesis().Measurements, types.SourcePumpDump, metrics,
		)

		for _, expectation := range baseline {
			metric := expectation.metric
			So(calm[metric], ShouldHaveLength, len(market.Symbols))

			for _, symbol := range market.Symbols {
				expected := expectation.expected

				if reference, ok := expected.(metricReference); ok {
					expected = calm[reference.metric][symbol]
				}

				if maximum, ok := expected.(metricMaximum); ok {
					expected = 0.0

					for _, source := range maximum {
						expected = max(expected.(float64), calm[source][symbol])
					}
				}

				So(calm[metric][symbol], expectation.assert, expected)
			}
		}

		for _, testCase := range cases {
			Convey(testCase.name, func() {
				peaks := make([]metricValues, 0, len(testCase.states))
				latest := make([]metricValues, 0, len(testCase.states))

				for _, state := range testCase.states {
					measurements := []*types.Measurement{}
					So(market.Transition(state, func() error {
						err := wired.Crypto.Tick()
						measurements = append(
							measurements,
							wired.Crypto.Thesis().Measurements...,
						)
						return err
					}), ShouldBeNil)
					peaks = append(peaks, utils.PeakMeasurements(measurements, types.SourcePumpDump, metrics))
					latest = append(latest, utils.LatestMeasurements(
						wired.Crypto.Thesis().Measurements,
						types.SourcePumpDump,
						metrics,
					))
				}

				for _, assertions := range []struct {
					actual       metricValues
					expectations []metricExpectation
				}{
					{peaks[len(peaks)-1], testCase.peaks},
					{latest[len(latest)-1], testCase.latest},
				} {
					for _, expectation := range assertions.expectations {
						metric := expectation.metric
						So(assertions.actual[metric], ShouldHaveLength, len(market.Symbols))

						for _, symbol := range market.Symbols {
							expected := expectation.expected

							if reference, ok := expected.(metricReference); ok {
								var values metricValues

								switch reference.step {
								case baselineStep:
									values = calm
								case currentStep:
									values = assertions.actual
								default:
									values = peaks[reference.step]
								}

								expected = values[reference.metric][symbol]
							}

							if maximum, ok := expected.(metricMaximum); ok {
								expected = 0.0

								for _, source := range maximum {
									expected = max(
										expected.(float64),
										assertions.actual[source][symbol],
									)
								}
							}

							So(assertions.actual[metric][symbol], expectation.assert, expected)
						}
					}
				}
			})
		}
	})
}

/*
BenchmarkMeasure exercises one full production tick against generated markets.
*/
func BenchmarkMeasure(b *testing.B) {
	market := tests.NewMarket(b.Context(), 3)

	if err := market.Bootstrap(); err != nil {
		b.Fatal(err)
	}

	wired, err := stack.NewBooter(b.Context()).Test(market)

	if err != nil {
		b.Fatal(err)
	}

	defer wired.Close()
	defer market.Close()
	b.ReportAllocs()
	state := tests.MarketStateFastPump

	for b.Loop() {
		if err := market.Transition(state, wired.Crypto.Tick); err != nil {
			b.Fatal(err)
		}

		if state == tests.MarketStateFastPump {
			state = tests.MarketStateFastDump
			continue
		}

		state = tests.MarketStateFastPump
	}
}
