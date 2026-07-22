package sentiment_test

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
TestCalculate proves sentiment separates broad conviction from isolated
leadership through the complete production boot graph.
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
	families := []types.MetricType{
		types.MetricSurgeScore,
		types.MetricDivergentScore,
		types.MetricSlumpScore,
	}

	Convey("Given cohort and isolated market tapes", t, func() {
		outcomes := map[string]map[types.MetricType]map[string]float64{}

		for _, proof := range []struct {
			name    string
			symbols []string
		}{
			{"cohort pump", nil},
			{"isolated pump", []string{"SIM1/USD"}},
		} {
			market := tests.NewMarket(t.Context(), 3)
			So(market.Bootstrap(), ShouldBeNil)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(wired.Crypto.Tick), ShouldBeNil)
			So(market.Transition(
				tests.MarketStateFastPump,
				wired.Crypto.Tick,
				proof.symbols...,
			), ShouldBeNil)
			outcomes[proof.name] = utils.LatestMeasurements(
				wired.Crypto.Thesis().Measurements,
				types.SourceSentiment,
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
					So(math.IsNaN(values[metric][symbol]), ShouldBeFalse)
				}

				So(values[types.MetricBreadth][symbol], ShouldBeBetweenOrEqual, 0, 1)

				for _, metric := range families {
					So(values[metric][symbol], ShouldBeGreaterThanOrEqualTo, 0)
					strength = max(strength, values[metric][symbol])
				}

				So(values[types.MetricStrength][symbol], ShouldEqual, strength)
			}
		}

		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			So(outcomes["cohort pump"][types.MetricBreadth][symbol], ShouldEqual, 1)
			So(outcomes["cohort pump"][types.MetricDivergentScore][symbol], ShouldEqual, 0)
			So(outcomes["cohort pump"][types.MetricSlumpScore][symbol], ShouldEqual, 0)
		}
		subject := "SIM1/USD"
		So(outcomes["isolated pump"][types.MetricBreadth][subject], ShouldBeLessThan, 1)
		So(outcomes["isolated pump"][types.MetricRelativeLead][subject], ShouldEqual, 1)
		So(outcomes["isolated pump"][types.MetricDivergentScore][subject], ShouldBeGreaterThan, 0)
	})
}
