package graph

import (
	"context"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/relation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
testCoordinate builds a minimal coordinate identity for the graph tests.
*/
func testCoordinate(source string, metric string) relation.Coordinate {
	return relation.Coordinate{
		Symbol:    "TEST/USD",
		Source:    source,
		Metric:    metric,
		Unit:      nmtypes.UnitDimensionless,
		Timescale: nmtypes.TimescaleInstantaneous,
		Epoch:     1,
	}
}

/*
testEdge builds an Influence edge with a defined Result so it participates in
the estimated lifecycle.
*/
func testEdge(source relation.Coordinate, target relation.Coordinate, coefficient float64, at time.Time) *InfluenceEdge {
	lag := time.Second

	return &InfluenceEdge{
		Type:   EdgeInfluence,
		Source: source,
		Target: target,
		Result: &relation.InfluenceResult{
			Source:           source,
			Target:           target,
			Lag:              lag,
			Coefficient:      &coefficient,
			From:             at.Add(-lag),
			At:               at,
			Maturity:         0.9,
			Status:           relation.FitOK,
			EstimatorVersion: "prequential-linear-v1",
		},
		From:  at.Add(-lag),
		At:    at,
		Epoch: 1,
	}
}

/*
testMeasurement builds one cvd Measurement carrying the two coordinates the
test plan relates. It lets the solver's Step exercise a real estimate path
without importing the strategy package (which imports this one).
*/
func testMeasurement(index int) *nmtypes.Measurement {
	at := time.Unix(0, int64(index)*int64(time.Second))

	return &nmtypes.Measurement{
		ID:           "test:" + strconv.Itoa(index),
		Source:       "cvd",
		Symbol:       "TEST/USD",
		At:           at,
		ObservedFrom: at.Add(-time.Second),
		Metrics: map[string]*nmtypes.Metric[float64]{
			"signed_net_fraction_zscore": nmtypes.NewMetric(
				"signed_net_fraction_zscore", float64(index%7),
				nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
			),
			"midpoint_log_return": nmtypes.NewMetric(
				"midpoint_log_return", float64(index%3),
				nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
			),
		},
	}
}

/*
testPlans builds a single RelationPlan that relates the signed-flow coordinate
to the return coordinate, so Step has a structurally eligible pair to estimate.
*/
func testPlans() []*relation.RelationPlan {
	source := testCoordinate("cvd", "signed_net_fraction_zscore")
	target := testCoordinate("cvd", "midpoint_log_return")

	return []*relation.RelationPlan{{
		Version: 1,
		Epoch:   1,
		Pairs: []relation.PlannedPair{{
			Source: relation.Selector{Source: source.Source, Metric: source.Metric},
			Target: relation.Selector{Source: target.Source, Metric: target.Metric},
		}},
		Lag: relation.LagDomain{MaxLag: 30 * time.Second},
	}}
}

func TestEdgeTypeString(t *testing.T) {
	Convey("Given the edge types", t, func() {
		Convey("Influence renders as influence", func() {
			So(EdgeInfluence.String(), ShouldEqual, "influence")
		})

		Convey("Association renders as association", func() {
			So(EdgeAssociation.String(), ShouldEqual, "association")
		})
	})
}

func TestCandidateStateString(t *testing.T) {
	Convey("Given the candidate states", t, func() {
		Convey("scheduled renders as candidate", func() {
			So(CandidateScheduled.String(), ShouldEqual, "candidate")
		})

		Convey("estimated renders as estimated", func() {
			So(CandidateEstimated.String(), ShouldEqual, "estimated")
		})

		Convey("unavailable renders as unavailable", func() {
			So(CandidateUnavailable.String(), ShouldEqual, "unavailable")
		})
	})
}

func TestNewInfluenceGraph(t *testing.T) {
	Convey("Given a new influence graph", t, func() {
		graph := NewInfluenceGraph(1, 2, 3, 8)

		Convey("it reports its identity", func() {
			So(graph.Epoch(), ShouldEqual, uint64(1))
		})

		Convey("it starts empty", func() {
			So(graph.NodeCount(), ShouldEqual, 0)
			So(graph.EdgeCount(), ShouldEqual, 0)
			So(graph.Nodes(), ShouldBeEmpty)
			So(graph.Edges(), ShouldBeEmpty)
			So(graph.Candidates(), ShouldBeEmpty)
		})

		Convey("a sub-one history capacity is clamped to one", func() {
			clamped := NewInfluenceGraph(1, 2, 3, 0)
			So(clamped.historyCapacity, ShouldEqual, 1)
		})
	})
}

func TestInfluenceGraphUpsertEdge(t *testing.T) {
	Convey("Given an empty influence graph", t, func() {
		graph := NewInfluenceGraph(1, 1, 1, 8)
		source := testCoordinate("cvd", "signed_net_fraction_zscore")
		target := testCoordinate("cvd", "midpoint_log_return")
		at := time.Unix(0, 149*int64(time.Second))

		Convey("a valid edge registers nodes and becomes queryable", func() {
			err := graph.UpsertEdge(testEdge(source, target, 0.004, at))
			So(err, ShouldBeNil)

			So(graph.NodeCount(), ShouldEqual, 2)
			So(graph.EdgeCount(), ShouldEqual, 1)
			So(graph.Relation(source, target), ShouldNotBeNil)
		})

		Convey("history accumulates chronologically rather than overwriting", func() {
			_ = graph.UpsertEdge(testEdge(source, target, 0.001, at))
			_ = graph.UpsertEdge(testEdge(source, target, 0.002, at.Add(time.Second)))

			history := graph.History(source, target)
			So(history, ShouldHaveLength, 2)
			So(*history[0].Result.Coefficient, ShouldEqual, 0.001)
			So(*history[1].Result.Coefficient, ShouldEqual, 0.002)
		})

		Convey("nil graph and nil edge are rejected", func() {
			var nilGraph *InfluenceGraph
			So(nilGraph.UpsertEdge(testEdge(source, target, 1, at)), ShouldNotBeNil)
			So(graph.UpsertEdge(nil), ShouldNotBeNil)
		})

		Convey("an incompatible epoch is rejected", func() {
			edge := testEdge(source, target, 0.004, at)
			edge.Epoch = 99
			So(graph.UpsertEdge(edge), ShouldNotBeNil)
		})
	})
}

func TestInfluenceGraphCandidateLifecycle(t *testing.T) {
	Convey("Given an empty influence graph", t, func() {
		graph := NewInfluenceGraph(1, 1, 1, 8)
		source := testCoordinate("cvd", "signed_net_fraction_zscore")
		target := testCoordinate("cvd", "midpoint_log_return")

		Convey("registering a candidate schedules it", func() {
			err := graph.RegisterCandidate(EdgeInfluence, source, target, 1)
			So(err, ShouldBeNil)

			candidates := graph.Candidates()
			So(candidates, ShouldHaveLength, 1)
			So(candidates[0].State, ShouldEqual, CandidateScheduled)
		})

		Convey("marking an unregistered candidate unavailable fails", func() {
			err := graph.SetUnavailable(EdgeInfluence, source, target, 1)
			So(err, ShouldNotBeNil)
		})

		Convey("a registered candidate marked unavailable stays visible", func() {
			_ = graph.RegisterCandidate(EdgeInfluence, source, target, 1)
			err := graph.SetUnavailable(EdgeInfluence, source, target, 1)
			So(err, ShouldBeNil)

			candidates := graph.Candidates()
			So(candidates, ShouldHaveLength, 1)
			So(candidates[0].State, ShouldEqual, CandidateUnavailable)
		})

		Convey("an estimated edge reflects the estimated state", func() {
			at := time.Unix(0, 149*int64(time.Second))
			_ = graph.UpsertEdge(testEdge(source, target, 0.004, at))

			candidates := graph.Candidates()
			So(candidates, ShouldHaveLength, 1)
			So(candidates[0].State, ShouldEqual, CandidateEstimated)
		})
	})
}

func TestInfluenceGraphQueries(t *testing.T) {
	Convey("Given a graph with measured relations", t, func() {
		graph := NewInfluenceGraph(1, 1, 1, 8)
		flow := testCoordinate("cvd", "signed_net_fraction_zscore")
		returnCoordinate := testCoordinate("cvd", "midpoint_log_return")
		gross := testCoordinate("cvd", "gross_notional_rate_zscore")
		at := time.Unix(0, 149*int64(time.Second))

		_ = graph.UpsertEdge(testEdge(flow, returnCoordinate, 0.004, at))
		_ = graph.UpsertEdge(testEdge(gross, flow, 0.05, at))

		Convey("incoming and outgoing are directional", func() {
			So(graph.Outgoing(flow), ShouldHaveLength, 1)
			So(graph.Outgoing(returnCoordinate), ShouldBeEmpty)

			So(graph.Incoming(returnCoordinate), ShouldHaveLength, 1)
			So(graph.Incoming(gross), ShouldBeEmpty)
		})

		Convey("relation returns the directed edge when present", func() {
			So(graph.Relation(flow, returnCoordinate), ShouldNotBeNil)
			So(graph.Relation(returnCoordinate, flow), ShouldBeNil)
		})

		Convey("edges and nodes retain full relation statistics", func() {
			edges := graph.Edges()
			So(edges, ShouldHaveLength, 2)

			nodes := graph.Nodes()
			So(nodes, ShouldHaveLength, 3)
		})
	})
}

func TestInfluenceGraphHistoryCapacity(t *testing.T) {
	Convey("Given a graph with bounded history", t, func() {
		graph := NewInfluenceGraph(1, 1, 1, 2)
		source := testCoordinate("cvd", "signed_net_fraction_zscore")
		target := testCoordinate("cvd", "midpoint_log_return")
		at := time.Unix(0, 0)

		Convey("retention is bounded by the capacity", func() {
			for index := 0; index < 5; index++ {
				_ = graph.UpsertEdge(testEdge(source, target, float64(index), at.Add(time.Duration(index)*time.Second)))
			}

			So(graph.History(source, target), ShouldHaveLength, 2)
		})
	})
}

func TestPlanVersion(t *testing.T) {
	Convey("Given relation plans", t, func() {
		Convey("the highest version wins", func() {
			plans := []*relation.RelationPlan{
				{Version: 1},
				{Version: 3},
				{Version: 2},
			}
			So(planVersion(plans), ShouldEqual, uint64(3))
		})

		Convey("nil plans are skipped", func() {
			plans := []*relation.RelationPlan{nil, {Version: 5}}
			So(planVersion(plans), ShouldEqual, uint64(5))
		})

		Convey("empty input yields zero", func() {
			So(planVersion(nil), ShouldEqual, uint64(0))
		})
	})
}

func TestSolverStep(t *testing.T) {
	Convey("Given a solver", t, func() {
		solver := NewSolver(context.Background(), 1, 2048, testPlans(), 1)

		Convey("stepping measurements appends observations and maintains the graph in place", func() {
			for index := 0; index < 120; index++ {
				update := solver.StepMeasurement(testMeasurement(index))
				So(update, ShouldNotBeNil)
			}

			Convey("the coordinate store retained observations", func() {
				So(solver.Store().Snapshot().Observations, ShouldBeGreaterThan, 0)
			})

			Convey("the influence graph has nodes and candidates", func() {
				So(solver.Graph().NodeCount(), ShouldBeGreaterThan, 0)
				So(solver.Graph().Candidates(), ShouldNotBeEmpty)
			})
		})
	})
}

func TestSolverNameAndAccessors(t *testing.T) {
	Convey("Given a nil solver", t, func() {
		var solver *Solver

		Convey("store and graph accessors are nil-safe", func() {
			So(solver.Store(), ShouldBeNil)
			So(solver.Graph(), ShouldBeNil)
		})

		Convey("step is nil-safe", func() {
			So(solver.StepMeasurement(nil), ShouldBeNil)
		})
	})
}

func TestInfluenceGraphWire(t *testing.T) {
	Convey("Given a graph with an estimated edge and a store with observations", t, func() {
		graph := NewInfluenceGraph(1, 1, 1, 8)
		source := testCoordinate("cvd", "signed_net_fraction_zscore")
		target := testCoordinate("cvd", "midpoint_log_return")
		at := time.Unix(0, 149*int64(time.Second))

		_ = graph.UpsertEdge(testEdge(source, target, 0.004, at))

		store := relation.NewObservationStore(8)
		store.AppendObservations([]relation.Observation{
			{
				Coordinate: source,
				Raw:        -1.5,
				From:       at.Add(-time.Second),
				At:         at,
				Maturity:   0.9,
			},
			{
				Coordinate: target,
				Raw:        2.25,
				From:       at.Add(-time.Second),
				At:         at,
				Maturity:   0.7,
			},
		})

		Convey("nodes are stamped with their latest observation", func() {
			frame := InfluenceGraphWire(store, graph, "TEST/USD", at)
			So(frame, ShouldNotBeNil)
			So(frame.Nodes, ShouldHaveLength, 2)

			var node *wire.GraphNodeT

			for _, candidate := range frame.Nodes {
				if candidate.Id == source.ID() {
					node = candidate
				}
			}

			So(node, ShouldNotBeNil)
			So(node.Kind, ShouldEqual, "measurement")
			So(node.Value, ShouldEqual, -1.5)
			So(node.Strength, ShouldEqual, 1.5)
			So(node.Confidence, ShouldEqual, 0.9)
			So(node.At, ShouldEqual, at.UnixNano())
		})

		Convey("edges carry the fitted relation statistics", func() {
			frame := InfluenceGraphWire(store, graph, "TEST/USD", at)
			So(frame.Edges, ShouldHaveLength, 1)
			So(frame.Edges[0].Weight, ShouldEqual, 0.004)
			So(frame.Edges[0].Confidence, ShouldEqual, 0.9)
			So(frame.Edges[0].Relation, ShouldEqual, "influence")
			So(frame.Edges[0].Reason, ShouldContainSubstring, "prequential-linear-v1")
		})

		Convey("nodes outside the focused symbol are excluded", func() {
			frame := InfluenceGraphWire(store, graph, "OTHER/USD", at)
			So(frame.Nodes, ShouldBeEmpty)
			So(frame.Edges, ShouldBeEmpty)
		})

		Convey("a coordinate without observations stays an identity node", func() {
			unobserved := testCoordinate("depthflow", "book_imbalance")
			_ = graph.UpsertEdge(testEdge(unobserved, source, 0.01, at))

			frame := InfluenceGraphWire(store, graph, "TEST/USD", at)
			So(frame.Nodes, ShouldHaveLength, 3)

			var identity *wire.GraphNodeT

			for _, node := range frame.Nodes {
				if node.Id == unobserved.ID() {
					identity = node
				}
			}

			So(identity, ShouldNotBeNil)
			So(identity.Value, ShouldEqual, 0)
			So(identity.At, ShouldEqual, 0)
		})

		Convey("a nil store leaves nodes as pure identity", func() {
			frame := InfluenceGraphWire(nil, graph, "TEST/USD", at)
			So(frame, ShouldNotBeNil)
			So(frame.Nodes, ShouldHaveLength, 2)
			So(frame.Nodes[0].Value, ShouldEqual, 0)
			So(frame.Nodes[0].At, ShouldEqual, 0)
		})

		Convey("a nil graph yields no frame", func() {
			So(InfluenceGraphWire(store, nil, "TEST/USD", at), ShouldBeNil)
		})
	})

	Convey("Given a graph with only scheduled candidates and no fitted edges", t, func() {
		graph := NewInfluenceGraph(1, 1, 1, 8)
		source := testCoordinate("depthflow", "book_imbalance")
		target := testCoordinate("cvd", "signed_net_fraction_zscore")

		_ = graph.RegisterCandidate(EdgeInfluence, source, target, 1)

		Convey("the frame still renders the structural candidates, not an empty graph", func() {
			frame := InfluenceGraphWire(nil, graph, "TEST/USD", time.Unix(0, 0))
			So(frame, ShouldNotBeNil)

			So(frame.Nodes, ShouldHaveLength, 2)
			So(frame.Edges, ShouldHaveLength, 1)

			edge := frame.Edges[0]
			So(edge.From, ShouldEqual, source.ID())
			So(edge.To, ShouldEqual, target.ID())
			So(edge.Reason, ShouldEqual, "state=candidate")
			So(edge.Derived, ShouldBeTrue)
			So(edge.Weight, ShouldEqual, 0)
			So(edge.Confidence, ShouldEqual, 0)
		})
	})
}

/*
Benchmarks mirror the exported hot-path methods so the lock-free storage cost is
measurable against iterations.
*/
func BenchmarkUpsertEdge(b *testing.B) {
	graph := NewInfluenceGraph(1, 1, 1, 64)
	source := testCoordinate("cvd", "signed_net_fraction_zscore")
	target := testCoordinate("cvd", "midpoint_log_return")
	at := time.Unix(0, 0)
	edge := testEdge(source, target, 0.004, at)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = graph.UpsertEdge(edge)
	}
}

func BenchmarkRelationLookup(b *testing.B) {
	graph := NewInfluenceGraph(1, 1, 1, 64)
	source := testCoordinate("cvd", "signed_net_fraction_zscore")
	target := testCoordinate("cvd", "midpoint_log_return")
	at := time.Unix(0, 0)
	_ = graph.UpsertEdge(testEdge(source, target, 0.004, at))

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = graph.Relation(source, target)
	}
}

func BenchmarkStepMeasurement(b *testing.B) {
	solver := NewSolver(context.Background(), 1, 2048, testPlans(), 1)

	measurement := testMeasurement(0)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = solver.StepMeasurement(measurement)
	}
}
