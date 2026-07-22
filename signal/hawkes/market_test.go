package hawkes_test

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
TestCalculate proves every empirical and fitted Hawkes metric survives slow
and clustered fixture arrivals through the complete production boot graph.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricEventCount,
		types.MetricArrivalRate,
		types.MetricConditionalIntensity,
		types.MetricBaselineIntensity,
		types.MetricExcitationAmplitude,
		types.MetricDecayRate,
		types.MetricKernelMemory,
		types.MetricSpectralRadius,
		types.MetricHawkesPoissonDelta,
		types.MetricCrossSelfDelta,
		types.MetricImmediateOffspring,
		types.MetricTotalDescendants,
	}

	Convey("Given slow and clustered production-booted arrival tapes", t, func() {
		outcomes := map[string]map[types.MetricType]map[string]float64{}

		for _, proof := range []struct {
			name  string
			state tests.MarketState
		}{
			{"slow arrivals", tests.MarketStateSlowPump},
			{"clustered arrivals", tests.MarketStateFastPump},
		} {
			market := tests.NewMarket(t.Context(), 3)
			So(market.Bootstrap(), ShouldBeNil)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(wired.Crypto.Tick), ShouldBeNil)
			measurements := []*types.Measurement{}
			So(market.Transition(proof.state, func() error {
				err := wired.Crypto.Tick()
				measurements = append(measurements, wired.Crypto.Thesis().Measurements...)
				return err
			}), ShouldBeNil)
			outcomes[proof.name] = utils.PeakMeasurements(
				measurements,
				types.SourceHawkes,
				metrics,
			)
			wired.Close()
			market.Close()
		}

		for _, values := range outcomes {
			for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
				for _, metric := range metrics {
					So(values[metric], ShouldContainKey, symbol)
					So(math.IsNaN(values[metric][symbol]), ShouldBeFalse)
					So(math.IsInf(values[metric][symbol], 0), ShouldBeFalse)
				}

				for _, metric := range []types.MetricType{
					types.MetricEventCount,
					types.MetricArrivalRate,
					types.MetricConditionalIntensity,
					types.MetricBaselineIntensity,
					types.MetricExcitationAmplitude,
					types.MetricDecayRate,
					types.MetricKernelMemory,
					types.MetricSpectralRadius,
					types.MetricImmediateOffspring,
					types.MetricTotalDescendants,
				} {
					So(values[metric][symbol], ShouldBeGreaterThanOrEqualTo, 0)
				}

				So(values[types.MetricEventCount][symbol], ShouldBeGreaterThan, 0)
				So(values[types.MetricArrivalRate][symbol], ShouldBeGreaterThan, 0)
				So(values[types.MetricDecayRate][symbol], ShouldBeGreaterThan, 0)
				So(values[types.MetricKernelMemory][symbol], ShouldBeGreaterThan, 0)
			}
		}

		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			So(outcomes["clustered arrivals"][types.MetricArrivalRate][symbol],
				ShouldBeGreaterThan,
				outcomes["slow arrivals"][types.MetricArrivalRate][symbol])
		}
	})
}
