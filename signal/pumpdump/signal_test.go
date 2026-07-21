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

/*
TestMeasure proves a fixture-driven pump reaches the real pumpdump
signal through the production boot graph and produces ignition for every
simulated symbol.
*/
func TestMeasure(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricRVOL,
		types.MetricPrecursor,
		types.MetricSpread,
		types.MetricCompression,
		types.MetricIgnition,
		types.MetricTrend,
		types.MetricExhaustion,
		types.MetricStrength,
	}
	cases := []struct {
		name   string
		states []tests.MarketState
		assert func(metricValues, []metricValues, []metricValues, []string)
	}{
		{
			name:   "When the market transitions into a fast pump it should detect ignition",
			states: []tests.MarketState{tests.MarketStateFastPump},
			assert: func(calm metricValues, peaks, _ []metricValues, symbols []string) {
				pump := peaks[0]

				for _, symbol := range symbols {
					So(pump[types.MetricRVOL][symbol], ShouldBeGreaterThan, calm[types.MetricRVOL][symbol])
					So(pump[types.MetricPrecursor][symbol], ShouldBeGreaterThan, calm[types.MetricPrecursor][symbol])
					So(pump[types.MetricSpread][symbol], ShouldAlmostEqual, calm[types.MetricSpread][symbol])
					So(pump[types.MetricCompression][symbol], ShouldEqual, 0)
					So(pump[types.MetricIgnition][symbol], ShouldBeGreaterThan, calm[types.MetricIgnition][symbol])
					So(pump[types.MetricTrend][symbol], ShouldBeGreaterThan, calm[types.MetricTrend][symbol])
					So(pump[types.MetricExhaustion][symbol], ShouldBeGreaterThanOrEqualTo, 0)
					So(pump[types.MetricStrength][symbol], ShouldEqual, pump[types.MetricIgnition][symbol])
				}
			},
		},
		{
			name: "When a fast pump transitions into a fast dump it should replace lift with rejection",
			states: []tests.MarketState{
				tests.MarketStateFastPump,
				tests.MarketStateFastDump,
			},
			assert: func(_ metricValues, peaks, latest []metricValues, symbols []string) {
				pump, dump := peaks[0], peaks[1]

				for _, symbol := range symbols {
					So(dump[types.MetricRVOL][symbol], ShouldBeGreaterThan, 0)
					So(latest[1][types.MetricRVOL][symbol], ShouldBeLessThan, pump[types.MetricRVOL][symbol])
					So(dump[types.MetricPrecursor][symbol], ShouldBeLessThan, pump[types.MetricPrecursor][symbol])
					So(dump[types.MetricSpread][symbol], ShouldAlmostEqual, pump[types.MetricSpread][symbol])
					So(dump[types.MetricCompression][symbol], ShouldEqual, 0)
					So(dump[types.MetricIgnition][symbol], ShouldBeLessThan, pump[types.MetricIgnition][symbol])
					So(dump[types.MetricTrend][symbol], ShouldBeLessThan, pump[types.MetricTrend][symbol])
					So(dump[types.MetricExhaustion][symbol], ShouldBeGreaterThan, 0)
					So(dump[types.MetricStrength][symbol], ShouldEqual, dump[types.MetricExhaustion][symbol])
				}
			},
		},
		{
			name: "When a fast pump transitions into a slow dump it should detect pump exhaustion",
			states: []tests.MarketState{
				tests.MarketStateFastPump,
				tests.MarketStateSlowDump,
			},
			assert: func(_ metricValues, peaks, _ []metricValues, symbols []string) {
				pump, dump := peaks[0], peaks[1]

				for _, symbol := range symbols {
					So(dump[types.MetricRVOL][symbol], ShouldBeLessThan, pump[types.MetricRVOL][symbol])
					So(dump[types.MetricPrecursor][symbol], ShouldBeLessThan, pump[types.MetricPrecursor][symbol])
					So(dump[types.MetricSpread][symbol], ShouldAlmostEqual, pump[types.MetricSpread][symbol])
					So(dump[types.MetricCompression][symbol], ShouldEqual, 0)
					So(dump[types.MetricIgnition][symbol], ShouldBeLessThan, pump[types.MetricIgnition][symbol])
					So(dump[types.MetricTrend][symbol], ShouldBeLessThan, pump[types.MetricTrend][symbol])
					So(dump[types.MetricExhaustion][symbol], ShouldBeGreaterThan, 0)
					So(dump[types.MetricStrength][symbol], ShouldEqual, dump[types.MetricExhaustion][symbol])
					So(dump[types.MetricStrength][symbol], ShouldBeLessThan, pump[types.MetricStrength][symbol])
				}
			},
		},
		{
			name:   "When the market transitions into a slow pump it should detect sustained ignition",
			states: []tests.MarketState{tests.MarketStateSlowPump},
			assert: func(calm metricValues, peaks, _ []metricValues, symbols []string) {
				pump := peaks[0]

				for _, symbol := range symbols {
					So(pump[types.MetricRVOL][symbol], ShouldBeGreaterThan, 0)
					So(pump[types.MetricPrecursor][symbol], ShouldBeGreaterThan, calm[types.MetricPrecursor][symbol])
					So(pump[types.MetricSpread][symbol], ShouldAlmostEqual, calm[types.MetricSpread][symbol])
					So(pump[types.MetricCompression][symbol], ShouldEqual, 0)
					So(pump[types.MetricIgnition][symbol], ShouldBeGreaterThan, calm[types.MetricIgnition][symbol])
					So(pump[types.MetricTrend][symbol], ShouldBeGreaterThan, calm[types.MetricTrend][symbol])
					So(pump[types.MetricExhaustion][symbol], ShouldBeGreaterThanOrEqualTo, 0)
					So(pump[types.MetricStrength][symbol], ShouldEqual, pump[types.MetricIgnition][symbol])
				}
			},
		},
		{
			name:   "When the market transitions into a slow dump it should not classify rejection as ignition",
			states: []tests.MarketState{tests.MarketStateSlowDump},
			assert: func(calm metricValues, peaks, _ []metricValues, symbols []string) {
				dump := peaks[0]

				for _, symbol := range symbols {
					So(dump[types.MetricRVOL][symbol], ShouldBeGreaterThan, 0)
					So(dump[types.MetricPrecursor][symbol], ShouldEqual, 0)
					So(dump[types.MetricSpread][symbol], ShouldAlmostEqual, calm[types.MetricSpread][symbol])
					So(dump[types.MetricCompression][symbol], ShouldEqual, 0)
					So(dump[types.MetricIgnition][symbol], ShouldEqual, 0)
					So(dump[types.MetricTrend][symbol], ShouldEqual, 0)
					So(dump[types.MetricExhaustion][symbol], ShouldBeGreaterThan, 0)
					So(dump[types.MetricStrength][symbol], ShouldEqual, dump[types.MetricExhaustion][symbol])
				}
			},
		},
		{
			name: "When a slow dump reverses into a fast pump it should retain rejection until clearing the baseline",
			states: []tests.MarketState{
				tests.MarketStateSlowDump,
				tests.MarketStateFastPump,
			},
			assert: func(_ metricValues, peaks, _ []metricValues, symbols []string) {
				rejection, recovery := peaks[0], peaks[1]

				for _, symbol := range symbols {
					So(recovery[types.MetricRVOL][symbol], ShouldBeGreaterThan, rejection[types.MetricRVOL][symbol])
					So(recovery[types.MetricPrecursor][symbol], ShouldEqual, 0)
					So(recovery[types.MetricSpread][symbol], ShouldAlmostEqual, rejection[types.MetricSpread][symbol])
					So(recovery[types.MetricCompression][symbol], ShouldEqual, 0)
					So(recovery[types.MetricIgnition][symbol], ShouldEqual, 0)
					So(recovery[types.MetricTrend][symbol], ShouldEqual, 0)
					So(recovery[types.MetricExhaustion][symbol], ShouldBeGreaterThan, 0)
					So(recovery[types.MetricStrength][symbol], ShouldEqual, recovery[types.MetricExhaustion][symbol])
				}
			},
		},
	}

	Convey("Given a baseline market state", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)

		Reset(func() {
			wired.Close()
			market.Close()
		})

		tests.WithMarket(market, []tests.MarketState{tests.MarketStateBaseline}, func() {
			So(wired.Crypto.Tick(), ShouldBeNil)
			calm := utils.LatestMeasurements(
				wired.Crypto.Thesis().Measurements,
				types.SourcePumpDump,
				metrics,
			)

			for _, metric := range metrics {
				So(calm[metric], ShouldHaveLength, len(market.Symbols))
			}

			for _, symbol := range market.Symbols {
				So(calm[types.MetricRVOL][symbol], ShouldBeGreaterThan, 0)
				So(calm[types.MetricPrecursor][symbol], ShouldBeGreaterThanOrEqualTo, 0)
				So(calm[types.MetricSpread][symbol], ShouldBeGreaterThan, 0)
				So(calm[types.MetricCompression][symbol], ShouldEqual, 0)
				So(calm[types.MetricIgnition][symbol], ShouldBeGreaterThanOrEqualTo, 0)
				So(calm[types.MetricTrend][symbol], ShouldBeGreaterThanOrEqualTo, 0)
				So(calm[types.MetricExhaustion][symbol], ShouldBeGreaterThanOrEqualTo, 0)
				So(calm[types.MetricStrength][symbol], ShouldBeGreaterThanOrEqualTo, calm[types.MetricIgnition][symbol])
			}

			for _, testCase := range cases {
				Convey(testCase.name, func() {
					peaks := make([]metricValues, 0, len(testCase.states))
					latest := make([]metricValues, 0, len(testCase.states))

					for _, state := range testCase.states {
						market.Transition(state)
						So(wired.Crypto.Tick(), ShouldBeNil)
						measurements := wired.Crypto.Thesis().Measurements
						peaks = append(peaks, utils.PeakMeasurements(
							measurements,
							types.SourcePumpDump,
							metrics,
						))
						latest = append(latest, utils.LatestMeasurements(
							measurements,
							types.SourcePumpDump,
							metrics,
						))
					}

					for _, metric := range metrics {
						So(peaks[len(peaks)-1][metric], ShouldHaveLength, len(market.Symbols))
					}

					testCase.assert(calm, peaks, latest, market.Symbols)
				})
			}
		})()
	})
}

/*
BenchmarkMeasure exercises one full production tick against generated markets.
*/
func BenchmarkMeasure(b *testing.B) {
	market := tests.NewMarket(b.Context(), 3)
	wired, err := stack.NewBooter(b.Context()).Test(market)

	if err != nil {
		b.Fatal(err)
	}

	defer wired.Close()
	defer market.Close()
	b.ReportAllocs()
	state := tests.MarketStateFastPump

	for b.Loop() {
		market.Transition(state)

		if err := wired.Crypto.Tick(); err != nil {
			b.Fatal(err)
		}

		if state == tests.MarketStateFastPump {
			state = tests.MarketStateFastDump
			continue
		}

		state = tests.MarketStateFastPump
	}
}
