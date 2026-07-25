package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/types"
)

func TestTrapShare(t *testing.T) {
	Convey("Given thesis measurements for one symbol", t, func() {
		at := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
		thesis := types.NewThesis()

		Convey("When only trap mass is present", func() {
			absorption := 0.8
			thesis.Publish(types.SourceCVD, []*types.Measurement{{
				Source: types.SourceCVD,
				Symbol: "SIM1/USD",
				At:     at,
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricAbsorption, types.SideNone): {
						Raw: absorption, Normalized: &absorption,
					},
				},
			}})

			evidence := TrapShare(thesis, "SIM1/USD")

			Convey("It reports full trap share", func() {
				So(evidence.Share, ShouldEqual, 1)
				So(evidence.Family, ShouldEqual, string(types.MetricAbsorption))
				So(evidence.TrapMass, ShouldEqual, absorption)
				So(evidence.OpportunityMass, ShouldEqual, 0)
			})
		})

		Convey("When only opportunity mass is present", func() {
			ignition := 0.6
			thesis.Publish(types.SourcePumpDump, []*types.Measurement{{
				Source: types.SourcePumpDump,
				Symbol: "SIM1/USD",
				At:     at,
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricIgnition, types.SideNone): {
						Raw: ignition, Normalized: &ignition,
					},
				},
			}})

			evidence := TrapShare(thesis, "SIM1/USD")

			Convey("It reports zero trap share", func() {
				So(evidence.Share, ShouldEqual, 0)
				So(evidence.TrapMass, ShouldEqual, 0)
				So(evidence.OpportunityMass, ShouldEqual, ignition)
			})
		})

		Convey("When trap and opportunity masses both exist", func() {
			spoof := 0.9
			drive := 0.3
			thesis.Publish(types.SourceDepthFlow, []*types.Measurement{{
				Source: types.SourceDepthFlow,
				Symbol: "SIM1/USD",
				At:     at,
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricSpoofScore, types.SideNone): {
						Raw: spoof, Normalized: &spoof,
					},
				},
			}})
			thesis.Publish(types.SourceCVD, []*types.Measurement{{
				Source: types.SourceCVD,
				Symbol: "SIM1/USD",
				At:     at,
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricDrive, types.SideNone): {
						Raw: drive, Normalized: &drive,
					},
				},
			}})

			evidence := TrapShare(thesis, "SIM1/USD")

			Convey("It reports the mass ratio without static cutoffs", func() {
				So(evidence.Share, ShouldAlmostEqual, spoof/(spoof+drive))
				So(evidence.Family, ShouldEqual, string(types.MetricSpoofScore))
				So(evidence.Dominates(), ShouldBeTrue)
			})
		})

		Convey("When no usable masses exist", func() {
			evidence := TrapShare(thesis, "SIM1/USD")

			Convey("It claims no trap share", func() {
				So(evidence.Share, ShouldEqual, 0)
				So(evidence.Dominates(), ShouldBeFalse)
			})
		})
	})
}

func TestCategoryTrap(t *testing.T) {
	Convey("Given a thesis with a resident category graph", t, func() {
		thesis := types.NewThesis()
		graph := category.NewGraph()
		at := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
		graph.Update(at, thesis, []types.Category{
			{Symbol: "SIM1/USD", Type: types.SpoofTrap, Strength: 0.9, Freshness: 1},
			{Symbol: "SIM1/USD", Type: types.VerticalIgnition, Strength: 0.3, Freshness: 1},
		})
		thesis.Graphs.Store("categories", graph)

		Convey("When trap structure outweighs opportunity", func() {
			share, dominates := CategoryTrap(thesis, "SIM1/USD")

			Convey("It reports dominance for strategy veto", func() {
				So(share, ShouldBeGreaterThan, 0.5)
				So(dominates, ShouldBeTrue)
			})
		})
	})
}

func BenchmarkTrapShare(b *testing.B) {
	at := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	absorption := 0.5
	ignition := 0.5
	thesis := types.NewThesis()
	thesis.Publish(types.SourceCVD, []*types.Measurement{{
		Source: types.SourceCVD,
		Symbol: "SIM1/USD", At: at,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricAbsorption, types.SideNone): {
				Raw: absorption, Normalized: &absorption,
			},
		},
	}})
	thesis.Publish(types.SourcePumpDump, []*types.Measurement{{
		Source: types.SourcePumpDump,
		Symbol: "SIM1/USD", At: at,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricIgnition, types.SideNone): {
				Raw: ignition, Normalized: &ignition,
			},
		},
	}})

	b.ReportAllocs()

	for b.Loop() {
		_ = TrapShare(thesis, "SIM1/USD")
	}
}
