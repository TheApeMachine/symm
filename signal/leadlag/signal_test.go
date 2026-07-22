package leadlag_test

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
marketOutcome retains transient, settled, and directed peer evidence from one
lead-lag tape because the directed metric is intentionally sparse.
*/
type marketOutcome struct {
	peak       metricValues
	latest     metricValues
	directions []*types.Measurement
}

/*
TestCalculate proves lead-lag distinguishes synchronization, inefficiency,
decoupling, stalls, and directed peers through the production boot graph.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricCorrelation,
		types.MetricSignedCorrelation,
		types.MetricSignedContempCorrelation,
		types.MetricSignedLagCorrelation,
		types.MetricLagFraction,
		types.MetricSampleSupport,
		types.MetricInefficient,
		types.MetricSync,
		types.MetricDecoupled,
		types.MetricStall,
		types.MetricStrength,
	}
	families := metrics[6:10]

	Convey("Given synchronized, delayed, rejecting, and isolated market tapes", t, func() {
		proofs := []struct {
			name    string
			states  []tests.MarketState
			symbols []string
		}{
			{"baseline", []tests.MarketState{tests.MarketStateBaseline}, nil},
			{"fast pump", []tests.MarketState{tests.MarketStateFastPump}, nil},
			{"slow pump", []tests.MarketState{tests.MarketStateSlowPump}, nil},
			{"fast dump", []tests.MarketState{tests.MarketStateFastDump}, nil},
			{"slow dump", []tests.MarketState{tests.MarketStateSlowDump}, nil},
			{"absorption", []tests.MarketState{tests.MarketStateVolumeAbsorption}, nil},
			{"compression", []tests.MarketState{tests.MarketStateSpreadCompression}, nil},
			{"loaded", []tests.MarketState{tests.MarketStateLoadedLiquidity}, nil},
			{"leader follower", []tests.MarketState{tests.MarketStateLeaderFollower}, nil},
			{"fast rejection", []tests.MarketState{
				tests.MarketStateFastPump, tests.MarketStateFastDump,
			}, nil},
			{"isolated pump", []tests.MarketState{tests.MarketStateFastPump}, []string{"SIM1/USD"}},
		}
		outcomes := make(map[string]marketOutcome, len(proofs))

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), 3)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(tests.Consume(wired.Crypto.Tick)), ShouldBeNil)
			measurements := []*types.Measurement{}

			for index, state := range proof.states {
				capture := index == len(proof.states)-1
				So(market.Transition(state, func() error {
					thesis, err := wired.Crypto.Tick()

					if err != nil {
						return err
					}

					if capture {
						measurements = append(measurements, thesis.Measurements...)
					}

					return nil
				}, proof.symbols...), ShouldBeNil)
			}

			directions := []*types.Measurement{}

			for _, measurement := range measurements {
				if measurement.Source == types.SourceLeadLag &&
					measurement.Metric == types.MetricSignedLagDirection {
					directions = append(directions, measurement)
				}
			}

			outcomes[proof.name] = marketOutcome{
				peak: utils.PeakMeasurements(measurements, types.SourceLeadLag, metrics),
				latest: utils.LatestMeasurements(
					measurements, types.SourceLeadLag, metrics,
				),
				directions: directions,
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
					strength := 0.0

					for _, metric := range metrics {
						So(math.IsNaN(values[metric][symbol]), ShouldBeFalse)
						So(math.IsInf(values[metric][symbol], 0), ShouldBeFalse)
					}

					So(values[types.MetricCorrelation][symbol], ShouldBeBetweenOrEqual, 0, 1)
					So(values[types.MetricSignedCorrelation][symbol], ShouldBeBetweenOrEqual, -1, 1)
					So(values[types.MetricSignedContempCorrelation][symbol], ShouldBeBetweenOrEqual, -1, 1)
					So(values[types.MetricSignedLagCorrelation][symbol], ShouldBeBetweenOrEqual, -1, 1)
					So(values[types.MetricLagFraction][symbol], ShouldBeBetweenOrEqual, 0, 1)
					So(values[types.MetricSampleSupport][symbol], ShouldBeBetweenOrEqual, 0, 1)

					for _, metric := range families {
						So(values[metric][symbol], ShouldBeBetweenOrEqual, 0, 1)
						strength = max(strength, values[metric][symbol])
					}

					So(values[types.MetricStrength][symbol], ShouldEqual, strength)
				}
			}

			for _, direction := range outcome.directions {
				So(direction.Peer, ShouldNotBeEmpty)
				So(math.Abs(direction.Raw), ShouldEqual, 1)
			}
		}

		for _, inactive := range []string{"baseline", "loaded"} {
			for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
				for _, metric := range families {
					So(outcomes[inactive].latest[metric][symbol], ShouldEqual, 0)
				}
			}
		}

		for _, cohort := range []string{
			"fast pump", "slow pump", "fast dump", "slow dump", "fast rejection",
		} {
			synchronized := 0

			for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
				sync := outcomes[cohort].latest[types.MetricSync][symbol]

				if sync == 0 {
					continue
				}

				synchronized++
				So(outcomes[cohort].latest[types.MetricLagFraction][symbol], ShouldEqual, 0)
				So(sync, ShouldBeGreaterThan,
					outcomes[cohort].latest[types.MetricInefficient][symbol])
				So(sync, ShouldBeGreaterThan,
					outcomes[cohort].latest[types.MetricDecoupled][symbol])
				So(sync, ShouldBeGreaterThan,
					outcomes[cohort].latest[types.MetricStall][symbol])
			}

			So(synchronized, ShouldEqual, 2)
		}

		delayed := outcomes["leader follower"].latest
		So(delayed[types.MetricStrength]["SIM1/USD"], ShouldEqual, 0)

		for _, follower := range []string{"SIM2/USD", "SIM3/USD"} {
			So(math.Abs(delayed[types.MetricSignedLagCorrelation][follower]),
				ShouldBeGreaterThan,
				math.Abs(delayed[types.MetricSignedContempCorrelation][follower]))
		}

		So(delayed[types.MetricLagFraction]["SIM3/USD"], ShouldBeGreaterThan,
			delayed[types.MetricLagFraction]["SIM2/USD"])
		latestDirectionAt := time.Time{}

		for _, direction := range outcomes["leader follower"].directions {
			if direction.At.After(latestDirectionAt) {
				latestDirectionAt = direction.At
			}
		}

		latestDirections := map[string]*types.Measurement{}

		for _, direction := range outcomes["leader follower"].directions {
			if direction.At.Equal(latestDirectionAt) {
				latestDirections[direction.Symbol] = direction
			}
		}

		So(latestDirections, ShouldHaveLength, 2)

		for _, follower := range []string{"SIM2/USD", "SIM3/USD"} {
			So(latestDirections[follower].Peer, ShouldEqual, "SIM1/USD")
			So(latestDirections[follower].Raw, ShouldEqual, 1)
		}

		isolated := outcomes["isolated pump"].latest
		So(isolated[types.MetricStrength]["SIM1/USD"], ShouldEqual, 0)

		for _, control := range []string{"SIM2/USD", "SIM3/USD"} {
			So(isolated[types.MetricDecoupled][control], ShouldBeGreaterThan,
				isolated[types.MetricSync][control])
			So(isolated[types.MetricSignedContempCorrelation][control], ShouldBeLessThan, 0)
		}
	})
}
