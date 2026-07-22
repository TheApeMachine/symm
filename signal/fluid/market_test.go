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
TestCalculate proves every Fluid score and physical reading is emitted for
directional and compressed tapes through the complete production boot graph.
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

	Convey("Given directional and compressed production-booted tapes", t, func() {
		outcomes := map[string]map[types.MetricType]map[string]float64{}

		for _, proof := range []struct {
			name  string
			state tests.MarketState
		}{
			{"directional", tests.MarketStateFastPump},
			{"compressed", tests.MarketStateSpreadCompression},
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
				types.SourceFluid,
				metrics,
			)
			wired.Close()
			market.Close()
		}

		for _, values := range outcomes {
			for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
				familyPeak := 0.0

				for _, metric := range metrics {
					So(values[metric], ShouldContainKey, symbol)
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
				So(values[types.MetricMemory][symbol], ShouldBeGreaterThanOrEqualTo, 0)
				So(values[types.MetricMidAddRate][symbol], ShouldBeGreaterThanOrEqualTo, 0)
				So(values[types.MetricMidExecuteRate][symbol], ShouldBeGreaterThanOrEqualTo, 0)
			}
		}
	})
}
