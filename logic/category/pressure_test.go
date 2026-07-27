package category

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestTrapPressure(t *testing.T) {
	Convey("Given a graph with trap and opportunity nodes", t, func() {
		graph := NewGraph()
		graph.touched = map[edgeKey]struct{}{}
		at := time.Unix(100, 0).UTC()

		graph.NodeIndex[nodeKey{symbol: "SIM/USD", kind: types.SpoofTrap}] = &Node{
			Symbol: "SIM/USD", Type: types.SpoofTrap, Strength: 0.9, At: at,
		}
		graph.Nodes = append(graph.Nodes, graph.NodeIndex[nodeKey{symbol: "SIM/USD", kind: types.SpoofTrap}])

		graph.NodeIndex[nodeKey{symbol: "SIM/USD", kind: types.VerticalIgnition}] = &Node{
			Symbol: "SIM/USD", Type: types.VerticalIgnition, Strength: 0.3, At: at,
		}
		graph.Nodes = append(graph.Nodes, graph.NodeIndex[nodeKey{symbol: "SIM/USD", kind: types.VerticalIgnition}])

		reporter := Report(graph)

		Convey("When trap mass exceeds opportunity mass", func() {
			share, dominates := reporter.TrapPressure("SIM/USD")

			Convey("It should report trap dominance", func() {
				So(dominates, ShouldBeTrue)
				So(share, ShouldBeGreaterThan, 0.5)
			})
		})
	})

	Convey("Given a graph with only opportunity nodes", t, func() {
		graph := NewGraph()
		graph.NodeIndex[nodeKey{symbol: "SIM/USD", kind: types.VerticalIgnition}] = &Node{
			Symbol: "SIM/USD", Type: types.VerticalIgnition, Strength: 0.8,
		}
		graph.Nodes = append(graph.Nodes, graph.NodeIndex[nodeKey{symbol: "SIM/USD", kind: types.VerticalIgnition}])

		reporter := Report(graph)
		share, dominates := reporter.TrapPressure("SIM/USD")

		Convey("It should not report trap dominance", func() {
			So(dominates, ShouldBeFalse)
			So(share, ShouldEqual, 0)
		})
	})

	Convey("Given a nil graph", t, func() {
		reporter := Report(nil)

		Convey("It should return zero without panicking", func() {
			share, dominates := reporter.TrapPressure("SIM/USD")
			So(share, ShouldEqual, 0)
			So(dominates, ShouldBeFalse)
		})
	})
}

func TestExhaustionLead(t *testing.T) {
	Convey("Given a graph with Leads edges into exhaustion and opportunity", t, func() {
		graph := NewGraph()
		graph.touched = map[edgeKey]struct{}{}

		graph.strengthen(
			time.Unix(1, 0).UTC(), "SIM/USD",
			types.VerticalIgnition, types.Exhaustion,
			Leads, 5.0, nil,
		)
		graph.strengthen(
			time.Unix(1, 0).UTC(), "SIM/USD",
			types.VerticalIgnition, types.OrganicTrend,
			Leads, 1.0, nil,
		)

		reporter := Report(graph)

		Convey("When exhaustion leads dominate", func() {
			share, dominates := reporter.ExhaustionLead("SIM/USD")

			Convey("It should report dominance with high share", func() {
				So(dominates, ShouldBeTrue)
				So(share, ShouldBeGreaterThan, 0.5)
			})
		})
	})
}

func TestOpportunityLead(t *testing.T) {
	Convey("Given a graph with Leads edges into opportunity categories", t, func() {
		graph := NewGraph()
		graph.touched = map[edgeKey]struct{}{}

		graph.strengthen(
			time.Unix(1, 0).UTC(), "SIM/USD",
			types.Exhaustion, types.VerticalIgnition,
			Leads, 5.0, nil,
		)
		graph.strengthen(
			time.Unix(1, 0).UTC(), "SIM/USD",
			types.Exhaustion, types.FadedExhaustion,
			Leads, 1.0, nil,
		)

		reporter := Report(graph)

		Convey("When opportunity leads dominate", func() {
			share, dominates := reporter.OpportunityLead("SIM/USD")

			Convey("It should report dominance", func() {
				So(dominates, ShouldBeTrue)
				So(share, ShouldBeGreaterThan, 0.5)
			})
		})
	})
}

func TestTokens(t *testing.T) {
	Convey("Given a graph with a known prior", t, func() {
		graph := NewGraph()
		graph.Priors["SIM/USD"] = types.OrganicTrend
		reporter := Report(graph)

		Convey("When the current top differs from the prior", func() {
			categories := []types.Category{
				{Symbol: "SIM/USD", Type: types.VerticalIgnition, Strength: 0.9},
			}

			tokens := reporter.Tokens("SIM/USD", categories)

			Convey("It should emit both prior and current as tokens", func() {
				So(tokens, ShouldContain, string(types.OrganicTrend))
				So(tokens, ShouldContain, string(types.VerticalIgnition))
			})
		})

		Convey("When the current top matches the prior", func() {
			categories := []types.Category{
				{Symbol: "SIM/USD", Type: types.OrganicTrend, Strength: 0.9},
			}

			tokens := reporter.Tokens("SIM/USD", categories)

			Convey("It should emit only the prior token", func() {
				So(len(tokens), ShouldEqual, 1)
				So(tokens[0], ShouldEqual, string(types.OrganicTrend))
			})
		})
	})

	Convey("Given a nil graph reporter", t, func() {
		reporter := Report(nil)

		Convey("It should return nil without panicking", func() {
			tokens := reporter.Tokens("SIM/USD", nil)
			So(tokens, ShouldBeNil)
		})
	})
}

func TestTop(t *testing.T) {
	Convey("Given a category slice with mixed types", t, func() {
		categories := []types.Category{
			{Type: types.CategoryTypeNone, Strength: 0.9},
			{Type: types.VerticalIgnition, Strength: 0.8},
			{Type: types.OrganicTrend, Strength: 0.5},
		}

		Convey("It should return the first non-None category", func() {
			top := Top(categories)
			So(top.Type, ShouldEqual, types.VerticalIgnition)
		})
	})

	Convey("Given an empty category slice", t, func() {
		Convey("It should return a zero Category", func() {
			top := Top(nil)
			So(top.Type, ShouldEqual, types.CategoryTypeNone)
		})
	})
}

func BenchmarkTrapPressure(b *testing.B) {
	graph := NewGraph()
	graph.touched = map[edgeKey]struct{}{}
	at := time.Unix(100, 0).UTC()

	trapTypes := []types.CategoryType{types.SpoofTrap, types.ToxicBluff, types.VolumeStarvation}
	oppTypes := []types.CategoryType{types.VerticalIgnition, types.OrganicTrend, types.RiskOnSurge}

	for _, categoryType := range trapTypes {
		key := nodeKey{symbol: "SIM/USD", kind: categoryType}
		node := &Node{Symbol: "SIM/USD", Type: categoryType, Strength: 0.7, At: at}
		graph.NodeIndex[key] = node
		graph.Nodes = append(graph.Nodes, node)
	}

	for _, categoryType := range oppTypes {
		key := nodeKey{symbol: "SIM/USD", kind: categoryType}
		node := &Node{Symbol: "SIM/USD", Type: categoryType, Strength: 0.5, At: at}
		graph.NodeIndex[key] = node
		graph.Nodes = append(graph.Nodes, node)
	}

	graph.strengthen(
		at, "SIM/USD", types.SpoofTrap, types.VerticalIgnition,
		Contradicts, 0.5, nil,
	)

	reporter := Report(graph)

	b.ReportAllocs()

	for b.Loop() {
		reporter.TrapPressure("SIM/USD")
	}
}

func BenchmarkExhaustionLead(b *testing.B) {
	graph := NewGraph()
	graph.touched = map[edgeKey]struct{}{}

	graph.strengthen(
		time.Unix(1, 0).UTC(), "SIM/USD",
		types.VerticalIgnition, types.Exhaustion,
		Leads, 5.0, nil,
	)
	graph.strengthen(
		time.Unix(1, 0).UTC(), "SIM/USD",
		types.VerticalIgnition, types.OrganicTrend,
		Leads, 1.0, nil,
	)

	reporter := Report(graph)

	b.ReportAllocs()

	for b.Loop() {
		reporter.ExhaustionLead("SIM/USD")
	}
}

func BenchmarkTokens(b *testing.B) {
	graph := NewGraph()
	graph.Priors["SIM/USD"] = types.OrganicTrend
	reporter := Report(graph)
	categories := []types.Category{
		{Symbol: "SIM/USD", Type: types.VerticalIgnition, Strength: 0.9},
	}

	b.ReportAllocs()

	for b.Loop() {
		reporter.Tokens("SIM/USD", categories)
	}
}
