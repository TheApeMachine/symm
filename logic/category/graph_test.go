package category

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestNewGraph(t *testing.T) {
	Convey("Given a freshly allocated graph", t, func() {
		graph := NewGraph()

		Convey("It should have empty but non-nil containers", func() {
			So(graph.Nodes, ShouldNotBeNil)
			So(graph.Edges, ShouldNotBeNil)
			So(graph.Priors, ShouldNotBeNil)
			So(graph.NodeIndex, ShouldNotBeNil)
			So(graph.EdgeIndex, ShouldNotBeNil)
			So(len(graph.Nodes), ShouldEqual, 0)
			So(len(graph.Edges), ShouldEqual, 0)
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Given an empty graph and thesis", t, func() {
		graph := NewGraph()
		thesis := types.NewThesis(nil)

		Convey("When Update receives categories for one symbol", func() {
			at := time.Unix(100, 0).UTC()
			categories := []types.Category{
				{
					Symbol: "SIM/USD", Type: types.VerticalIgnition,
					Strength: 0.8, Freshness: 1,
					Supporting: []string{string(types.MetricIgnition)},
				},
				{
					Symbol: "SIM/USD", Type: types.OrganicTrend,
					Strength: 0.6, Freshness: 1,
					Supporting: []string{string(types.MetricTrend)},
				},
			}

			graph.Update(at, thesis, categories)

			Convey("It should populate thesis categories and graph nodes", func() {
				So(len(thesis.Categories["SIM/USD"]), ShouldEqual, 2)
				So(len(graph.Nodes), ShouldEqual, 2)
				So(thesis.At, ShouldEqual, at)
			})

			Convey("It should record the prior for the symbol", func() {
				So(graph.Prior("SIM/USD"), ShouldEqual, types.VerticalIgnition)
			})
		})

		Convey("When Update receives a category with empty symbol", func() {
			at := time.Unix(100, 0).UTC()
			categories := []types.Category{
				{Symbol: "", Type: types.VerticalIgnition, Strength: 0.5},
			}

			graph.Update(at, thesis, categories)

			Convey("It should skip the empty symbol", func() {
				So(len(graph.Nodes), ShouldEqual, 0)
			})
		})
	})
}

func TestUpdateFrom(t *testing.T) {
	Convey("Given a graph with prior state from a first cut", t, func() {
		graph := NewGraph()
		thesis := types.NewThesis(nil)
		firstAt := time.Unix(100, 0).UTC()

		thesis.Categories["SIM/USD"] = []types.Category{
			{
				Symbol: "SIM/USD", Type: types.VerticalIgnition,
				Strength: 0.8, Freshness: 1,
				Supporting: []string{string(types.MetricIgnition)},
			},
		}
		thesis.At = firstAt
		graph.UpdateFrom(thesis)

		Convey("When a second cut activates a new category alongside the prior", func() {
			secondAt := firstAt.Add(2 * time.Second)

			thesis.Categories["SIM/USD"] = []types.Category{
				{
					Symbol: "SIM/USD", Type: types.VerticalIgnition,
					Strength: 0.9, Freshness: 1,
					Supporting: []string{string(types.MetricIgnition)},
				},
				{
					Symbol: "SIM/USD", Type: types.OrganicTrend,
					Strength: 0.6, Freshness: 1,
					Supporting: []string{string(types.MetricTrend)},
				},
			}
			thesis.At = secondAt
			graph.UpdateFrom(thesis)

			Convey("It should upsert both nodes", func() {
				So(len(graph.Nodes), ShouldEqual, 2)
			})

			Convey("It should derive edges between the pair", func() {
				So(len(graph.Edges), ShouldBeGreaterThan, 0)
			})

			Convey("It should record the new top as prior", func() {
				So(graph.Prior("SIM/USD"), ShouldEqual, types.VerticalIgnition)
			})
		})
	})
}

func TestStrengthen(t *testing.T) {
	Convey("Given a graph with touched initialized", t, func() {
		graph := NewGraph()
		graph.touched = map[edgeKey]struct{}{}
		at := time.Unix(1, 0).UTC()

		Convey("When strengthen is called for a new edge", func() {
			graph.strengthen(
				at, "SIM/USD",
				types.VerticalIgnition, types.OrganicTrend,
				Supports, 0.5,
				[]string{"ignition"},
			)

			Convey("It should create the relation in the index and slice", func() {
				So(len(graph.Edges), ShouldEqual, 1)
				So(graph.Edges[0].Weight, ShouldEqual, 0.5)
				So(graph.Edges[0].Type, ShouldEqual, Supports)
			})

			Convey("It should mark the edge as touched", func() {
				key := makeEdgeKey("SIM/USD", types.VerticalIgnition, types.OrganicTrend, Supports)
				_, ok := graph.touched[key]
				So(ok, ShouldBeTrue)
			})
		})

		Convey("When strengthen is called twice for the same edge", func() {
			graph.strengthen(
				at, "SIM/USD",
				types.VerticalIgnition, types.OrganicTrend,
				Supports, 0.3, []string{"a"},
			)
			graph.strengthen(
				at, "SIM/USD",
				types.VerticalIgnition, types.OrganicTrend,
				Supports, 0.2, []string{"b"},
			)

			Convey("It should accumulate weight on the same relation", func() {
				So(len(graph.Edges), ShouldEqual, 1)
				So(graph.Edges[0].Weight, ShouldAlmostEqual, 0.5)
			})

			Convey("It should replace evidence with the latest call", func() {
				So(graph.Edges[0].Evidence, ShouldResemble, []string{"b"})
			})
		})
	})
}

func TestStrengthenJoined(t *testing.T) {
	Convey("Given a graph with touched initialized", t, func() {
		graph := NewGraph()
		graph.touched = map[edgeKey]struct{}{}
		at := time.Unix(1, 0).UTC()

		Convey("When strengthenJoined is called", func() {
			graph.strengthenJoined(
				at, "SIM/USD",
				types.VerticalIgnition, types.OrganicTrend,
				Supports, 0.7,
				[]string{"a"}, []string{"b"},
			)

			Convey("It should create the relation with joined evidence", func() {
				So(len(graph.Edges), ShouldEqual, 1)
				So(graph.Edges[0].Evidence, ShouldResemble, []string{"a", "b"})
				So(graph.Edges[0].Weight, ShouldAlmostEqual, 0.7)
			})
		})
	})
}

func TestWeight(t *testing.T) {
	Convey("Given a nil graph", t, func() {
		var graph *Graph

		Convey("It should return zero without panicking", func() {
			So(graph.Weight("SIM/USD", types.VerticalIgnition, types.OrganicTrend, Supports), ShouldEqual, 0)
		})
	})

	Convey("Given a graph with one edge", t, func() {
		graph := NewGraph()
		graph.touched = map[edgeKey]struct{}{}
		graph.strengthen(
			time.Unix(1, 0).UTC(), "SIM/USD",
			types.VerticalIgnition, types.OrganicTrend,
			Supports, 1.5, nil,
		)

		Convey("It should return the weight for the matching key", func() {
			So(graph.Weight(
				"SIM/USD", types.VerticalIgnition, types.OrganicTrend, Supports,
			), ShouldAlmostEqual, 1.5)
		})

		Convey("It should return zero for a non-matching key", func() {
			So(graph.Weight(
				"SIM/USD", types.OrganicTrend, types.VerticalIgnition, Supports,
			), ShouldEqual, 0)
		})
	})
}

func TestPrior(t *testing.T) {
	Convey("Given a nil graph", t, func() {
		var graph *Graph

		Convey("It should return CategoryTypeNone without panicking", func() {
			So(graph.Prior("SIM/USD"), ShouldEqual, types.CategoryTypeNone)
		})
	})

	Convey("Given a graph after one update", t, func() {
		graph := NewGraph()
		thesis := types.NewThesis(nil)
		thesis.At = time.Unix(100, 0).UTC()
		thesis.Categories["SIM/USD"] = []types.Category{
			{Symbol: "SIM/USD", Type: types.OrganicTrend, Strength: 0.9, Freshness: 1},
		}
		graph.UpdateFrom(thesis)

		Convey("It should return the top category for the symbol", func() {
			So(graph.Prior("SIM/USD"), ShouldEqual, types.OrganicTrend)
		})

		Convey("It should return zero for an unknown symbol", func() {
			So(graph.Prior("UNKNOWN/USD"), ShouldEqual, types.CategoryTypeNone)
		})
	})
}

func BenchmarkStrengthen(b *testing.B) {
	graph := NewGraph()
	graph.touched = map[edgeKey]struct{}{}
	at := time.Unix(1, 0).UTC()
	evidence := []string{"ignition", "trend"}

	b.ReportAllocs()

	for b.Loop() {
		graph.strengthen(
			at, "SIM/USD",
			types.VerticalIgnition, types.OrganicTrend,
			Supports, 0.5, evidence,
		)
	}
}

func BenchmarkStrengthenJoined(b *testing.B) {
	graph := NewGraph()
	graph.touched = map[edgeKey]struct{}{}
	at := time.Unix(1, 0).UTC()
	first := []string{"ignition"}
	second := []string{"trend"}

	b.ReportAllocs()

	for b.Loop() {
		graph.strengthenJoined(
			at, "SIM/USD",
			types.VerticalIgnition, types.OrganicTrend,
			Supports, 0.5, first, second,
		)
	}
}

func BenchmarkWeight(b *testing.B) {
	graph := NewGraph()
	graph.touched = map[edgeKey]struct{}{}
	graph.strengthen(
		time.Unix(1, 0).UTC(), "SIM/USD",
		types.VerticalIgnition, types.OrganicTrend,
		Supports, 1.0, nil,
	)

	b.ReportAllocs()

	for b.Loop() {
		graph.Weight("SIM/USD", types.VerticalIgnition, types.OrganicTrend, Supports)
	}
}
