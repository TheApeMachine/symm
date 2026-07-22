package sentiment_test

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

type metricValues = map[types.MetricType]map[string]float64

/*
marketOutcome retains transition-local sentiment evidence so every semantic
expectation is checked against the tape that caused it.
*/
type marketOutcome struct {
	peak   metricValues
	latest metricValues
}

/*
TestCalculate proves sentiment distinguishes systemic advances, systemic
declines, neutral price noise, and isolated positive leadership through the
complete production boot graph.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricChange,
		types.MetricBreadth,
		types.MetricLeaderStrength,
		types.MetricLeaderEvidence,
		types.MetricRelativeLead,
		types.MetricSurgeScore,
		types.MetricDivergentScore,
		types.MetricSlumpScore,
		types.MetricStrength,
	}
	families := metrics[5:8]
	symbols := []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"}
	proofs := []struct {
		name    string
		states  []tests.MarketState
		symbols []string
		family  types.MetricType
		subject string
	}{
		{"baseline", []tests.MarketState{tests.MarketStateBaseline}, nil, "", ""},
		{"absorption", []tests.MarketState{tests.MarketStateVolumeAbsorption}, nil, "", ""},
		{"compression", []tests.MarketState{tests.MarketStateSpreadCompression}, nil, "", ""},
		{"fast pump", []tests.MarketState{tests.MarketStateFastPump}, nil, types.MetricSurgeScore, ""},
		{"slow pump", []tests.MarketState{tests.MarketStateSlowPump}, nil, types.MetricSurgeScore, ""},
		{"low-volume lift", []tests.MarketState{tests.MarketStateLowVolumeLift}, nil, types.MetricSurgeScore, ""},
		{"fast dump", []tests.MarketState{tests.MarketStateFastDump}, nil, types.MetricSlumpScore, ""},
		{"slow dump", []tests.MarketState{tests.MarketStateSlowDump}, nil, types.MetricSlumpScore, ""},
		{"isolated pump", []tests.MarketState{tests.MarketStateFastPump}, []string{"SIM1/USD"}, types.MetricDivergentScore, "SIM1/USD"},
	}

	Convey("Given directional, neutral, rejecting, and isolated market tapes", t, func() {
		outcomes := make(map[string]marketOutcome, len(proofs))

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), len(symbols))
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(wired.Crypto.Step), ShouldBeNil)
			measurements := []*types.Measurement{}

			for index, state := range proof.states {
				capture := index == len(proof.states)-1
				So(market.Transition(state, func() error {
					thesis, err := wired.Crypto.Tick()

					if err != nil {
						return err
					}

					if capture {
						measurements = append(
							measurements,
							thesis.Measurements...,
						)
					}

					return nil
				}, proof.symbols...), ShouldBeNil)
			}

			for _, measurement := range measurements {
				if measurement.Source != types.SourceSentiment {
					continue
				}

				So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
				So(measurement.Validity.Readiness, ShouldEqual, types.ReadinessObservation)
				So(measurement.Unit, ShouldEqual, types.UnitDimensionless)
			}

			outcomes[proof.name] = marketOutcome{
				peak: utils.PeakMeasurements(
					measurements,
					types.SourceSentiment,
					metrics,
				),
				latest: utils.LatestMeasurements(
					measurements,
					types.SourceSentiment,
					metrics,
				),
			}
			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		for _, proof := range proofs {
			outcome := outcomes[proof.name]
			views := []struct {
				name   string
				values metricValues
			}{
				{"peak", outcome.peak},
				{"latest", outcome.latest},
			}

			for _, view := range views {
				values := view.values
				for _, metric := range metrics {
					So(values[metric], ShouldHaveLength, len(symbols))
				}

				for _, symbol := range symbols {
					strength := 0.0

					for _, metric := range metrics {
						So(math.IsNaN(values[metric][symbol]), ShouldBeFalse)
						So(math.IsInf(values[metric][symbol], 0), ShouldBeFalse)
					}

					So(values[types.MetricBreadth][symbol], ShouldBeBetweenOrEqual, 0, 1)
					So(values[types.MetricLeaderStrength][symbol], ShouldBeGreaterThanOrEqualTo, 0)
					So(values[types.MetricLeaderEvidence][symbol], ShouldBeBetweenOrEqual, 0, 1)
					So(values[types.MetricRelativeLead][symbol], ShouldBeBetweenOrEqual, 0, 1)

					for _, family := range families {
						value := values[family][symbol]
						So(value, ShouldBeBetweenOrEqual, 0, 1)
						strength = max(strength, value)
						expected := family == proof.family &&
							(proof.subject == "" || proof.subject == symbol)

						claim := proof.name + " " + view.name + " " + symbol + " " + string(family)

						if expected {
							SoMsg(claim, value, ShouldBeGreaterThan, 0)
							continue
						}

						SoMsg(claim, value, ShouldEqual, 0)
					}

					So(values[types.MetricStrength][symbol], ShouldEqual, strength)
				}
			}

			for _, symbol := range symbols {
				switch proof.family {
				case types.MetricSurgeScore:
					So(outcome.latest[types.MetricChange][symbol], ShouldBeGreaterThan, 0)
					So(outcome.latest[types.MetricBreadth][symbol], ShouldEqual, 1)
				case types.MetricSlumpScore:
					So(outcome.latest[types.MetricChange][symbol], ShouldBeLessThan, 0)
					So(outcome.latest[types.MetricBreadth][symbol], ShouldEqual, 0)
				}
			}
		}

		isolated := outcomes["isolated pump"].latest

		for _, symbol := range symbols[1:] {
			So(isolated[types.MetricLeaderStrength][symbol], ShouldEqual, 0)
			So(isolated[types.MetricLeaderEvidence][symbol], ShouldEqual, 0)
			So(isolated[types.MetricRelativeLead][symbol], ShouldEqual, 0)
		}

		for _, metric := range metrics {
			for _, symbol := range symbols {
				So(outcomes["low-volume lift"].latest[metric][symbol], ShouldAlmostEqual,
					outcomes["fast pump"].latest[metric][symbol])
			}
		}
	})
}

/*
BenchmarkCalculate drives repeated ticker cuts through the complete production
graph so sentiment's cohort scan is measured with realistic decoded inputs.
*/
func BenchmarkCalculate(b *testing.B) {
	market := tests.NewMarket(b.Context(), 3)

	wired, err := stack.NewBooter(b.Context()).Test(market)

	if err != nil {
		b.Fatal(err)
	}

	defer func() {
		if err := wired.Close(); err != nil {
			b.Fatal(err)
		}
	}()
	defer market.Close()
	b.ReportAllocs()

	for b.Loop() {
		actions := make([]tests.MarketAction, 0, len(market.Symbols)*3)

		for _, symbol := range market.Symbols {
			actions = append(actions,
				tests.MarketAction{Kind: tests.MarketTrade, Symbol: symbol, Side: "buy", Qty: 10},
				tests.MarketAction{Kind: tests.MarketRefill, Symbol: symbol, Side: "sell", Qty: 10},
				tests.MarketAction{Kind: tests.MarketMoveMid, Symbol: symbol, Ticks: 1},
			)
		}

		if err := market.Apply(tests.MarketStep{
			Advance: time.Second,
			Actions: actions,
		}, wired.Crypto.Step); err != nil {
			b.Fatal(err)
		}
	}
}
