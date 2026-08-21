package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestActiveOpportunity(t *testing.T) {
	now := time.Unix(1_000, 0).UTC()
	originalCatalog := Catalog
	defer func() {
		Catalog = originalCatalog
	}()

	Convey("Given support and opposition that clear every declared floor", t, func() {
		Catalog = []OpportunityArchetype{{
			Type: OpportunitySuddenPump,
			Supports: []ObservationCondition{{
				Source: SourcePumpDump, Metric: string(MetricRVOL), Side: SideBuy,
				Supports: true, MaturityFloor: 0.4, SeparationFloor: 0.4,
			}},
			Opposes: []ObservationCondition{{
				Source: SourceToxicity, Metric: string(MetricBluffScore),
				Contradicts: true, MaturityFloor: 0.3, SeparationFloor: 0.3,
			}},
		}}
		graph := opportunityGraph(
			now,
			opportunityNode("support", "support-measurement", SourcePumpDump,
				MetricRVOL, SideBuy, 0.8, 0.5, now),
			opportunitySeparation("support-separation", "support-measurement",
				SourcePumpDump, 0.6, now),
			opportunityNode("opposition", "opposition-measurement", SourceToxicity,
				MetricBluffScore, SideNone, 0.5, 0.8, now),
			opportunitySeparation("opposition-separation", "opposition-measurement",
				SourceToxicity, 0.5, now),
		)

		active := graph.ActiveOpportunity(now)

		Convey("The exact trust-attenuated net evidence should be returned", func() {
			So(active.Type, ShouldEqual, OpportunitySuddenPump)
			So(active.Support, ShouldAlmostEqual, 0.24, 1e-12)
			So(active.Opposition, ShouldAlmostEqual, 0.20, 1e-12)
			So(active.Score, ShouldAlmostEqual, 0.04, 1e-12)
			So(active.Lifecycle, ShouldEqual, LifecycleEmergent)
		})
	})

	Convey("Given evidence with the wrong declared side", t, func() {
		Catalog = []OpportunityArchetype{{
			Type: OpportunitySuddenPump,
			Supports: []ObservationCondition{{
				Source: SourcePumpDump, Metric: string(MetricRVOL), Side: SideBuy,
				Supports: true, MaturityFloor: 0.4, SeparationFloor: 0.4,
			}},
		}}
		graph := opportunityGraph(
			now,
			opportunityNode("wrong-side", "measurement", SourcePumpDump,
				MetricRVOL, SideSell, 1, 1, now),
			opportunitySeparation("separation", "measurement", SourcePumpDump, 1, now),
		)

		Convey("It should carry no vote", func() {
			So(graph.ActiveOpportunity(now).Type, ShouldEqual, OpportunityNone)
		})
	})

	Convey("Given evidence below an epistemic floor", t, func() {
		Catalog = []OpportunityArchetype{{
			Type: OpportunitySuddenPump,
			Supports: []ObservationCondition{{
				Source: SourcePumpDump, Metric: string(MetricRVOL),
				Supports: true, MaturityFloor: 0.4, SeparationFloor: 0.4,
			}},
		}}

		Convey("Maturity below the floor should carry no vote", func() {
			graph := opportunityGraph(
				now,
				opportunityNode("immature", "measurement", SourcePumpDump,
					MetricRVOL, SideNone, 1, 0.39, now),
				opportunitySeparation("separation", "measurement", SourcePumpDump, 1, now),
			)

			So(graph.ActiveOpportunity(now).Type, ShouldEqual, OpportunityNone)
		})

		Convey("Separation below the floor should carry no vote", func() {
			graph := opportunityGraph(
				now,
				opportunityNode("ambiguous", "measurement", SourcePumpDump,
					MetricRVOL, SideNone, 1, 1, now),
				opportunitySeparation("separation", "measurement", SourcePumpDump, 0.39, now),
			)

			So(graph.ActiveOpportunity(now).Type, ShouldEqual, OpportunityNone)
		})

		Convey("Missing separation should carry no vote", func() {
			graph := opportunityGraph(
				now,
				opportunityNode("missing", "measurement", SourcePumpDump,
					MetricRVOL, SideNone, 1, 1, now),
			)

			So(graph.ActiveOpportunity(now).Type, ShouldEqual, OpportunityNone)
		})

		Convey("Separation from another measurement should carry no vote", func() {
			graph := opportunityGraph(
				now,
				opportunityNode("ambiguous", "measurement", SourcePumpDump,
					MetricRVOL, SideNone, 1, 1, now),
				opportunitySeparation("wrong-measurement", "other", SourcePumpDump, 1, now),
				opportunitySeparation("wrong-source", "measurement", SourceHawkes, 1, now),
			)

			So(graph.ActiveOpportunity(now).Type, ShouldEqual, OpportunityNone)
		})
	})

	Convey("Given catalog legs without the corresponding evidence role", t, func() {
		graph := opportunityGraph(
			now,
			opportunityNode("support", "support-measurement", SourcePumpDump,
				MetricRVOL, SideNone, 1, 1, now),
			opportunitySeparation("support-separation", "support-measurement",
				SourcePumpDump, 1, now),
			opportunityNode("opposition", "opposition-measurement", SourceToxicity,
				MetricBluffScore, SideNone, 1, 1, now),
			opportunitySeparation("opposition-separation", "opposition-measurement",
				SourceToxicity, 1, now),
		)

		Convey("An undeclared support role should not support", func() {
			Catalog = []OpportunityArchetype{{
				Type: OpportunitySuddenPump,
				Supports: []ObservationCondition{{
					Source: SourcePumpDump, Metric: string(MetricRVOL),
					MaturityFloor: 0.4, SeparationFloor: 0.4,
				}},
			}}

			So(graph.ActiveOpportunity(now).Type, ShouldEqual, OpportunityNone)
		})

		Convey("An undeclared contradiction role should not oppose", func() {
			Catalog = []OpportunityArchetype{{
				Type: OpportunitySuddenPump,
				Supports: []ObservationCondition{{
					Source: SourcePumpDump, Metric: string(MetricRVOL), Supports: true,
					MaturityFloor: 0.4, SeparationFloor: 0.4,
				}},
				Opposes: []ObservationCondition{{
					Source: SourceToxicity, Metric: string(MetricBluffScore),
					MaturityFloor: 0.4, SeparationFloor: 0.4,
				}},
			}}

			active := graph.ActiveOpportunity(now)
			So(active.Type, ShouldEqual, OpportunitySuddenPump)
			So(active.Support, ShouldEqual, 1.0)
			So(active.Opposition, ShouldEqual, 0.0)
			So(active.Score, ShouldEqual, 1.0)
		})
	})

	Convey("Given several matching readings and archetypes", t, func() {
		Catalog = []OpportunityArchetype{
			{
				Type: OpportunitySuddenPump,
				Supports: []ObservationCondition{{
					Source: SourcePumpDump, Metric: string(MetricRVOL), Supports: true,
					MaturityFloor: 0.4, SeparationFloor: 0.4,
				}},
			},
			{
				Type: OpportunityDailyRiser,
				Supports: []ObservationCondition{{
					Source: SourceSentiment, Metric: string(MetricBreadth), Supports: true,
					MaturityFloor: 0.4, SeparationFloor: 0.4,
				}},
			},
		}
		graph := opportunityGraph(
			now,
			opportunityNode("a-immature", "weak-measurement", SourcePumpDump,
				MetricRVOL, SideNone, 1, 0.1, now),
			opportunitySeparation("weak-separation", "weak-measurement",
				SourcePumpDump, 1, now),
			opportunityNode("b-qualified", "pump-measurement", SourcePumpDump,
				MetricRVOL, SideNone, 0.6, 1, now),
			opportunitySeparation("pump-separation", "pump-measurement",
				SourcePumpDump, 1, now),
			opportunityNode("riser", "riser-measurement", SourceSentiment,
				MetricBreadth, SideNone, 0.8, 1, now),
			opportunitySeparation("riser-separation", "riser-measurement",
				SourceSentiment, 1, now),
		)

		Convey("The strongest qualified positive archetype should win", func() {
			active := graph.ActiveOpportunity(now)
			So(active.Type, ShouldEqual, OpportunityDailyRiser)
			So(active.Score, ShouldAlmostEqual, 0.8, 1e-12)
		})
	})

	Convey("Given non-positive net evidence", t, func() {
		Catalog = []OpportunityArchetype{{
			Type: OpportunitySuddenPump,
			Supports: []ObservationCondition{{
				Source: SourcePumpDump, Metric: string(MetricRVOL), Supports: true,
				MaturityFloor: 0.4, SeparationFloor: 0.4,
			}},
			Opposes: []ObservationCondition{{
				Source: SourceToxicity, Metric: string(MetricBluffScore), Contradicts: true,
				MaturityFloor: 0.4, SeparationFloor: 0.4,
			}},
		}}
		Convey("Equal support and opposition should return none", func() {
			graph := opportunityGraph(
				now,
				opportunityNode("support", "support-measurement", SourcePumpDump,
					MetricRVOL, SideNone, 0.5, 1, now),
				opportunitySeparation("support-separation", "support-measurement",
					SourcePumpDump, 1, now),
				opportunityNode("opposition", "opposition-measurement", SourceToxicity,
					MetricBluffScore, SideNone, 0.5, 1, now),
				opportunitySeparation("opposition-separation", "opposition-measurement",
					SourceToxicity, 1, now),
			)

			active := graph.ActiveOpportunity(now)
			So(active.Type, ShouldEqual, OpportunityNone)
			So(active.Score, ShouldEqual, 0.0)
		})

		Convey("Stronger opposition should return none", func() {
			graph := opportunityGraph(
				now,
				opportunityNode("support", "support-measurement", SourcePumpDump,
					MetricRVOL, SideNone, 0.4, 1, now),
				opportunitySeparation("support-separation", "support-measurement",
					SourcePumpDump, 1, now),
				opportunityNode("opposition", "opposition-measurement", SourceToxicity,
					MetricBluffScore, SideNone, 0.6, 1, now),
				opportunitySeparation("opposition-separation", "opposition-measurement",
					SourceToxicity, 1, now),
			)

			active := graph.ActiveOpportunity(now)
			So(active.Type, ShouldEqual, OpportunityNone)
			So(active.Score, ShouldEqual, 0.0)
		})
	})
}

func opportunityGraph(now time.Time, nodes ...*Node) *Graph {
	graph := &Graph{At: now, Nodes: make(map[string]*Node, len(nodes))}

	for _, node := range nodes {
		graph.Nodes[node.ID] = node
	}

	return graph
}

func opportunityNode(
	id string,
	measurementID string,
	source SourceType,
	metric MetricType,
	side MeasurementSide,
	confidence float64,
	maturity float64,
	at time.Time,
) *Node {
	return &Node{
		ID: id, MeasurementID: measurementID, Source: string(source), Metric: metric,
		Side: side, Kind: KindMeasurement, Value: 1, Confidence: confidence,
		Maturity: maturity, At: at,
	}
}

func opportunitySeparation(
	id string,
	measurementID string,
	source SourceType,
	separation float64,
	at time.Time,
) *Node {
	return &Node{
		ID: id, MeasurementID: measurementID, Source: string(source),
		Metric: MetricHypothesisSeparation, Kind: KindMeasurement,
		Value: separation, Confidence: separation, Maturity: 1, At: at,
	}
}

func BenchmarkActiveOpportunity(b *testing.B) {
	now := time.Unix(1_000, 0).UTC()
	graph := opportunityGraph(
		now,
		opportunityNode("rvol", "pump", SourcePumpDump,
			MetricRVOL, SideNone, 0.8, 0.8, now),
		opportunitySeparation("pump-separation", "pump", SourcePumpDump, 0.8, now),
		opportunityNode("hawkes", "hawkes", SourceHawkes,
			MetricSpectralRadius, SideNone, 0.8, 0.8, now),
		opportunitySeparation("hawkes-separation", "hawkes", SourceHawkes, 0.8, now),
	)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = graph.ActiveOpportunity(now)
	}
}
