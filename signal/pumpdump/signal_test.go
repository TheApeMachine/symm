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
marketOutcome contains the strongest and final pumpdump evidence emitted by one
production-booted market tape.
*/
type marketOutcome struct {
	peak   metricValues
	latest metricValues
}

/*
marketProof names one semantic tape and any symbols isolated by that tape.
*/
type marketProof struct {
	name    string
	states  []tests.MarketState
	symbols []string
}

/*
metricComparison states one causal distinction the pumpdump signal must retain.
*/
type metricComparison struct {
	stronger string
	weaker   string
	metric   types.MetricType
}

/*
TestMeasure proves pumpdump distinguishes price lift, volume lift, compression,
rejection, and symbol isolation through the complete production boot graph.
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
	families := []types.MetricType{
		types.MetricCompression,
		types.MetricIgnition,
		types.MetricTrend,
		types.MetricExhaustion,
	}

	Convey("Given causal and adversarial market tapes", t, func() {
		proofs := []marketProof{
			{"fast pump", []tests.MarketState{tests.MarketStateFastPump}, nil},
			{"slow pump", []tests.MarketState{tests.MarketStateSlowPump}, nil},
			{"fast rejection", []tests.MarketState{
				tests.MarketStateFastPump,
				tests.MarketStateFastDump,
			}, nil},
			{"slow rejection", []tests.MarketState{
				tests.MarketStateFastPump,
				tests.MarketStateSlowDump,
			}, nil},
			{"slow dump", []tests.MarketState{tests.MarketStateSlowDump}, nil},
			{"reversal", []tests.MarketState{
				tests.MarketStateSlowDump,
				tests.MarketStateFastPump,
			}, nil},
			{"absorption", []tests.MarketState{tests.MarketStateVolumeAbsorption}, nil},
			{"low-volume lift", []tests.MarketState{tests.MarketStateLowVolumeLift}, nil},
			{"compression", []tests.MarketState{tests.MarketStateSpreadCompression}, nil},
			{"isolated pump", []tests.MarketState{tests.MarketStateFastPump}, []string{"SIM1/USD"}},
		}
		outcomes := make(map[string]marketOutcome, len(proofs))

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), 3)
			So(market.Bootstrap(), ShouldBeNil)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(wired.Crypto.Tick), ShouldBeNil)
			measurements := []*types.Measurement{}

			for index, state := range proof.states {
				capture := index == len(proof.states)-1
				So(market.Transition(state, func() error {
					err := wired.Crypto.Tick()

					if capture {
						measurements = append(
							measurements,
							wired.Crypto.Thesis().Measurements...,
						)
					}

					return err
				}, proof.symbols...), ShouldBeNil)
			}

			outcomes[proof.name] = marketOutcome{
				peak: utils.PeakMeasurements(
					measurements,
					types.SourcePumpDump,
					metrics,
				),
				latest: utils.LatestMeasurements(
					wired.Crypto.Thesis().Measurements,
					types.SourcePumpDump,
					metrics,
				),
			}
			wired.Close()
			market.Close()
		}

		for _, outcome := range outcomes {
			for _, values := range []metricValues{outcome.peak, outcome.latest} {
				for _, metric := range metrics {
					So(values[metric], ShouldHaveLength, 3)
				}

				for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
					So(values[types.MetricRVOL][symbol], ShouldBeGreaterThan, 0)
					So(values[types.MetricPrecursor][symbol], ShouldBeGreaterThanOrEqualTo, 0)
					So(values[types.MetricSpread][symbol], ShouldBeGreaterThan, 0)
					strength := 0.0

					for _, family := range families {
						So(values[family][symbol], ShouldBeGreaterThanOrEqualTo, 0)
						strength = max(strength, values[family][symbol])
					}

					So(values[types.MetricStrength][symbol], ShouldEqual, strength)
				}
			}
		}

		for _, comparison := range []metricComparison{
			{"fast pump", "slow pump", types.MetricRVOL},
			{"fast pump", "slow pump", types.MetricIgnition},
			{"fast pump", "slow pump", types.MetricStrength},
			{"fast pump", "absorption", types.MetricPrecursor},
			{"fast pump", "absorption", types.MetricIgnition},
			{"fast pump", "absorption", types.MetricTrend},
			{"fast pump", "absorption", types.MetricStrength},
			{"fast pump", "low-volume lift", types.MetricRVOL},
			{"fast pump", "low-volume lift", types.MetricIgnition},
			{"fast pump", "low-volume lift", types.MetricStrength},
			{"compression", "fast pump", types.MetricCompression},
			{"fast rejection", "fast pump", types.MetricExhaustion},
			{"slow rejection", "fast pump", types.MetricExhaustion},
			{"fast pump", "fast rejection", types.MetricPrecursor},
			{"fast pump", "fast rejection", types.MetricIgnition},
			{"fast pump", "fast rejection", types.MetricTrend},
			{"fast pump", "fast rejection", types.MetricStrength},
			{"fast pump", "slow rejection", types.MetricPrecursor},
			{"fast pump", "slow rejection", types.MetricIgnition},
			{"fast pump", "slow rejection", types.MetricTrend},
			{"fast pump", "slow rejection", types.MetricStrength},
			{"fast pump", "compression", types.MetricRVOL},
			{"fast pump", "compression", types.MetricPrecursor},
			{"fast pump", "compression", types.MetricIgnition},
			{"fast pump", "compression", types.MetricTrend},
			{"fast pump", "compression", types.MetricStrength},
			{"fast rejection", "compression", types.MetricExhaustion},
			{"reversal", "slow dump", types.MetricPrecursor},
			{"reversal", "slow dump", types.MetricIgnition},
			{"reversal", "slow dump", types.MetricTrend},
			{"reversal", "slow dump", types.MetricStrength},
		} {
			for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
				So(
					outcomes[comparison.stronger].peak[comparison.metric][symbol],
					ShouldBeGreaterThan,
					outcomes[comparison.weaker].peak[comparison.metric][symbol],
				)
			}
		}

		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			for _, name := range []string{"fast pump", "slow pump", "reversal"} {
				for _, metric := range []types.MetricType{
					types.MetricRVOL,
					types.MetricPrecursor,
					types.MetricIgnition,
					types.MetricTrend,
					types.MetricStrength,
				} {
					So(outcomes[name].peak[metric][symbol], ShouldBeGreaterThan, 1.0)
				}
			}

			for _, name := range []string{"fast rejection", "slow rejection", "slow dump"} {
				So(outcomes[name].peak[types.MetricExhaustion][symbol], ShouldBeGreaterThan, 0)

				for _, metric := range []types.MetricType{
					types.MetricPrecursor,
					types.MetricIgnition,
					types.MetricTrend,
					types.MetricStrength,
				} {
					So(outcomes[name].peak[metric][symbol], ShouldBeLessThan, 1.0)
				}
			}

			So(
				outcomes["absorption"].peak[types.MetricRVOL][symbol],
				ShouldAlmostEqual,
				outcomes["fast pump"].peak[types.MetricRVOL][symbol],
			)
			So(
				outcomes["compression"].latest[types.MetricSpread][symbol],
				ShouldBeLessThan,
				outcomes["fast pump"].latest[types.MetricSpread][symbol],
			)
		}

		isolated := outcomes["isolated pump"].peak

		for _, control := range []string{"SIM2/USD", "SIM3/USD"} {
			for _, metric := range []types.MetricType{
				types.MetricRVOL,
				types.MetricPrecursor,
				types.MetricIgnition,
				types.MetricTrend,
				types.MetricStrength,
			} {
				So(isolated[metric]["SIM1/USD"], ShouldBeGreaterThan, isolated[metric][control])
			}
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
