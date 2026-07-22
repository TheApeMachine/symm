package cvd_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
TestCalculate proves CVD distinguishes directional drive from high-volume
absorption while reporting the complete flow state through production boot.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricAbsorption,
		types.MetricDrive,
		types.MetricBalance,
		types.MetricStarvation,
		types.MetricStrength,
		types.MetricNetFraction,
		types.MetricNet,
	}
	families := []types.MetricType{
		types.MetricAbsorption,
		types.MetricDrive,
		types.MetricBalance,
		types.MetricStarvation,
	}

	Convey("Given production-booted directional and absorption tapes", t, func() {
		outcomes := map[string]map[types.MetricType]map[string]float64{}

		for _, proof := range []struct {
			name  string
			state tests.MarketState
		}{
			{"drive", tests.MarketStateFastPump},
			{"absorption", tests.MarketStateVolumeAbsorption},
		} {
			market := tests.NewMarket(t.Context(), 3)
			So(market.Bootstrap(), ShouldBeNil)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(wired.Crypto.Tick), ShouldBeNil)
			So(market.Transition(proof.state, wired.Crypto.Tick), ShouldBeNil)
			outcomes[proof.name] = utils.LatestMeasurements(
				wired.Crypto.Thesis().Measurements,
				types.SourceCVD,
				metrics,
			)
			wired.Close()
			market.Close()
		}

		for _, values := range outcomes {
			for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
				strength := 0.0

				for _, metric := range metrics {
					So(values[metric], ShouldContainKey, symbol)
				}

				for _, metric := range families {
					So(values[metric][symbol], ShouldBeGreaterThanOrEqualTo, 0)
					strength = max(strength, values[metric][symbol])
				}

				So(values[types.MetricStrength][symbol], ShouldEqual, strength)
				So(values[types.MetricNetFraction][symbol], ShouldBeBetweenOrEqual, -1, 1)
			}
		}

		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			So(outcomes["drive"][types.MetricDrive][symbol], ShouldBeGreaterThan,
				outcomes["drive"][types.MetricAbsorption][symbol])
			So(outcomes["absorption"][types.MetricAbsorption][symbol], ShouldBeGreaterThan,
				outcomes["drive"][types.MetricAbsorption][symbol])
			So(outcomes["drive"][types.MetricDrive][symbol], ShouldBeGreaterThan,
				outcomes["absorption"][types.MetricDrive][symbol])
		}
	})
}

/*
BenchmarkCalculate exercises repeated fixture-driven CVD transitions through
the complete production graph.
*/
func BenchmarkCalculate(b *testing.B) {
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

	for b.Loop() {
		if err := market.Transition(tests.MarketStateFastPump, wired.Crypto.Tick); err != nil {
			b.Fatal(err)
		}
	}
}
