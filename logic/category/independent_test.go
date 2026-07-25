package category

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestIndependentOf(t *testing.T) {
	Convey("Given co-active categories with an independence statistic", t, func() {
		graph := NewGraph()
		at := time.Unix(50, 0).UTC()
		thesis := types.NewThesis()
		thesis.At = at
		ignition := 0.8
		trend := 0.7
		decoupled := 0.9
		publishMetrics(thesis, types.SourcePumpDump, "SIM/USD", at, 2*time.Second, map[types.MetricType]types.MetricSample{
			types.MetricIgnition: {Raw: ignition, Normalized: &ignition},
			types.MetricTrend:    {Raw: trend, Normalized: &trend},
		})
		thesis.Publish(types.SourceLeadLag, []*types.Measurement{
			measurementWithMetric(
				types.SourceLeadLag, "SIM/USD", at,
				types.MetricDecoupled, decoupled, &decoupled, 2*time.Second,
			),
		})

		graph.Update(at, thesis, Compose(thesis, "SIM/USD"))

		Convey("It strengthens IndependentOf instead of Supports", func() {
			So(graph.Weight(
				"SIM/USD", types.VerticalIgnition, types.OrganicTrend, IndependentOf,
			), ShouldBeGreaterThan, 0)
			So(graph.Weight(
				"SIM/USD", types.VerticalIgnition, types.OrganicTrend, Supports,
			), ShouldEqual, 0)
		})
	})

	Convey("Given pair memory where joint mass sits below the product baseline", t, func() {
		memory := newPairMemory()
		memory.observe("SIM/USD", types.VerticalIgnition, 10)
		memory.observe("SIM/USD", types.OrganicTrend, 10)
		memory.coobserve("SIM/USD", types.VerticalIgnition, types.OrganicTrend, 0.1, 0.1)

		Convey("It reports independence mass", func() {
			mass, ok := memory.independent(
				"SIM/USD", types.VerticalIgnition, types.OrganicTrend, 0.5, 0.5,
			)
			So(ok, ShouldBeTrue)
			So(mass, ShouldBeGreaterThan, 0)
		})
	})
}
