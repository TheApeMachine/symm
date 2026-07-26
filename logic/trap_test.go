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
			thesis.AppendMeasurements([]*types.Measurement{{
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
			thesis.AppendMeasurements([]*types.Measurement{{
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
			thesis.AppendMeasurements([]*types.Measurement{{
				Source: types.SourceDepthFlow,
				Symbol: "SIM1/USD",
				At:     at,
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricSpoofScore, types.SideNone): {
						Raw: spoof, Normalized: &spoof,
					},
				},
			}})
			thesis.AppendMeasurements([]*types.Measurement{{
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
	thesis.AppendMeasurements([]*types.Measurement{{
		Source: types.SourceCVD,
		Symbol: "SIM1/USD", At: at,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricAbsorption, types.SideNone): {
				Raw: absorption, Normalized: &absorption,
			},
		},
	}})
	thesis.AppendMeasurements([]*types.Measurement{{
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

func TestCategoryOpportunityLead(t *testing.T) {
	Convey("Given a resident category graph with Leads edges", t, func() {
		thesis := types.NewThesis()
		graph := category.NewGraph()
		at := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)

		Convey("When Leads edges point into opportunity categories", func() {
			// Two-step activation: VerticalIgnition activates first, then
			// OrganicTrend activates — graph records Leads: Ignition→Trend.
			graph.Update(at, thesis, []types.Category{
				{Symbol: "SIM1/USD", Type: types.VerticalIgnition, Strength: 0.9, Freshness: 1},
			})
			graph.Update(at.Add(time.Second), thesis, []types.Category{
				{Symbol: "SIM1/USD", Type: types.VerticalIgnition, Strength: 0.9, Freshness: 1, Supporting: []string{"ignition"}},
				{Symbol: "SIM1/USD", Type: types.OrganicTrend, Strength: 0.7, Freshness: 1, Supporting: []string{"trend"}},
			})
			thesis.Graphs.Store("categories", graph)

			Convey("It reports positive opportunity lead share", func() {
				share, dominates := CategoryOpportunityLead(thesis, "SIM1/USD")
				So(share, ShouldBeGreaterThan, 0)
				So(dominates, ShouldBeTrue)
			})
		})

		Convey("When Leads edges point into exhaustion categories", func() {
			graph.Update(at, thesis, []types.Category{
				{Symbol: "SIM1/USD", Type: types.VerticalIgnition, Strength: 0.9, Freshness: 1},
			})
			graph.Update(at.Add(time.Second), thesis, []types.Category{
				{Symbol: "SIM1/USD", Type: types.VerticalIgnition, Strength: 0.9, Freshness: 1, Supporting: []string{"ignition"}},
				{Symbol: "SIM1/USD", Type: types.Exhaustion, Strength: 0.8, Freshness: 1, Supporting: []string{"exhaust"}},
			})
			thesis.Graphs.Store("categories", graph)

			Convey("It reports zero opportunity dominance", func() {
				_, dominates := CategoryOpportunityLead(thesis, "SIM1/USD")
				So(dominates, ShouldBeFalse)
			})
		})

		Convey("When no graph is on thesis", func() {
			Convey("It returns zero share without panicking", func() {
				share, dominates := CategoryOpportunityLead(thesis, "SIM1/USD")
				So(share, ShouldEqual, 0)
				So(dominates, ShouldBeFalse)
			})
		})
	})
}
