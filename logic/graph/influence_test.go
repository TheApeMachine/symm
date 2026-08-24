package graph

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/relation"
)

func testCoordinate(source string, metric string) relation.Coordinate {
	return relation.Coordinate{
		Symbol: "BTC/USD",
		Source: source,
		Metric: metric,
		Epoch:  1,
	}
}

func testEdge(
	edgeType EdgeType,
	source relation.Coordinate,
	target relation.Coordinate,
	gain float64,
	lag time.Duration,
	coefficient float64,
) *InfluenceEdge {
	gainValue := gain
	coefficientValue := coefficient
	variance := 0.01
	snr := coefficient * coefficient / variance

	return &InfluenceEdge{
		Type:   edgeType,
		Source: source,
		Target: target,
		Result: &relation.InfluenceResult{
			Source:          source,
			Target:          target,
			Lag:             lag,
			LagResolution:   time.Second,
			LagSearchSpan:   5 * time.Second,
			LagCandidateCount: 5,
			Coefficient:     &coefficientValue,
			CoefficientVariance: &variance,
			CoefficientSNR:  &snr,
			PredictiveGain:  &gainValue,
			EffectiveSampleCount: 200,
			Maturity:        0.995,
			EstimatorVersion: "test-v1",
			Epoch:           1,
			Status:          relation.FitOK,
		},
		From:  time.Unix(100, 0),
		At:    time.Unix(300, 0),
		Epoch: 1,
	}
}

func TestInfluenceGraphConformance(t *testing.T) {
	Convey("Given an influence graph", t, func() {
		influenceGraph := NewInfluenceGraph(1, 1, 1, 64)
		source := testCoordinate("cvd", "signed_net_fraction")
		target := testCoordinate("price", "midpoint_log_return")
		other := testCoordinate("hawkes", "conditional_intensity")

		Convey("zero-gain and low-SNR edges are retained, not pruned", func() {
			zeroGain := 0.0
			edge := testEdge(EdgeInfluence, source, target, 0, time.Second, 0.01)

			err := influenceGraph.UpsertEdge(edge)
			So(err, ShouldBeNil)

			retrieved := influenceGraph.Relation(source, target)
			So(retrieved, ShouldNotBeNil)
			So(*retrieved.Result.PredictiveGain, ShouldEqual, zeroGain)
			So(*retrieved.Result.Coefficient, ShouldEqual, 0.01)

			Convey("incoming and outgoing queries retain the relation statistics", func() {
				incoming := influenceGraph.Incoming(target)
				So(len(incoming), ShouldEqual, 1)
				So(incoming[0].Result, ShouldEqual, edge.Result)

				outgoing := influenceGraph.Outgoing(source)
				So(len(outgoing), ShouldEqual, 1)
			})
		})

		Convey("candidate but unavailable is not no relationship", func() {
			err := influenceGraph.RegisterCandidate(EdgeInfluence, source, target, 1)
			So(err, ShouldBeNil)

			err = influenceGraph.SetUnavailable(EdgeInfluence, source, target, 1)
			So(err, ShouldBeNil)

			candidates := influenceGraph.Candidates()
			So(len(candidates), ShouldEqual, 1)
			So(candidates[0].State, ShouldEqual, CandidateUnavailable)
			So(influenceGraph.Relation(source, target), ShouldBeNil)
		})

		Convey("association edges stay distinct from influence edges", func() {
			association := testEdge(EdgeAssociation, source, target, 0, 0, 0.5)

			err := influenceGraph.UpsertEdge(association)
			So(err, ShouldBeNil)

			So(influenceGraph.CurrentEdge(EdgeAssociation, source, target), ShouldNotBeNil)
			So(influenceGraph.Relation(source, target), ShouldBeNil)

			Convey("zero-lag association is never published as influence", func() {
				influence := influenceGraph.Relation(source, target)
				So(influence, ShouldBeNil)
			})
		})

		Convey("edge history is retained in chronological order", func() {
			first := testEdge(EdgeInfluence, source, target, 0.1, time.Second, 0.2)
			first.At = time.Unix(100, 0)
			second := testEdge(EdgeInfluence, source, target, 0.5, 2*time.Second, -0.3)
			second.At = time.Unix(200, 0)

			So(influenceGraph.UpsertEdge(first), ShouldBeNil)
			So(influenceGraph.UpsertEdge(second), ShouldBeNil)

			history := influenceGraph.History(source, target)
			So(len(history), ShouldEqual, 2)
			So(history[0].At, ShouldEqual, first.At)
			So(history[1].At, ShouldEqual, second.At)

			Convey("current values do not erase historical edge state", func() {
				current := influenceGraph.Relation(source, target)
				So(current.Result.Lag, ShouldEqual, 2*time.Second)
				So(*current.Result.Coefficient, ShouldEqual, -0.3)
				So(influenceGraph.History(source, target), ShouldHaveLength, 2)
			})
		})

		Convey("lag provenance survives serialization", func() {
			edge := testEdge(EdgeInfluence, source, target, 0.7, 3*time.Second, 0.4)

			encoded, err := json.Marshal(edge)
			So(err, ShouldBeNil)

			var decoded InfluenceEdge
			err = json.Unmarshal(encoded, &decoded)
			So(err, ShouldBeNil)
			So(decoded.Result.Lag, ShouldEqual, 3*time.Second)
			So(decoded.Result.LagResolution, ShouldEqual, time.Second)
			So(*decoded.Result.PredictiveGain, ShouldEqual, 0.7)
			So(*decoded.Result.Coefficient, ShouldEqual, 0.4)
			So(decoded.Result.Source, ShouldEqual, source)
			So(decoded.Result.EstimatorVersion, ShouldEqual, "test-v1")
		})

		Convey("family rollups expose the underlying coordinate edges", func() {
			So(influenceGraph.UpsertEdge(testEdge(EdgeInfluence, source, target, 0.3, time.Second, 0.2)), ShouldBeNil)
			So(influenceGraph.UpsertEdge(testEdge(EdgeInfluence, other, target, 0.4, time.Second, 0.1)), ShouldBeNil)

			rollup := influenceGraph.FamilyEdges(
				relation.Selector{Source: "cvd"},
				relation.Selector{Metric: "midpoint_log_return"},
			)
			So(len(rollup), ShouldEqual, 1)
			So(rollup[0].Source, ShouldEqual, source)

			allTowards := influenceGraph.FamilyEdges(
				relation.Selector{},
				relation.Selector{Metric: "midpoint_log_return"},
			)
			So(len(allTowards), ShouldEqual, 2)
		})

		Convey("epoch-incompatible edges are rejected, not silently merged", func() {
			edge := testEdge(EdgeInfluence, source, target, 0.2, time.Second, 0.1)
			edge.Epoch = 2

			err := influenceGraph.UpsertEdge(edge)
			So(err, ShouldNotBeNil)
			So(influenceGraph.Relation(source, target), ShouldBeNil)
		})

		Convey("paths between coordinates retain edge measurements", func() {
			mediator := testCoordinate("cvd", "net_notional_rate")
			So(influenceGraph.UpsertEdge(testEdge(EdgeInfluence, source, mediator, 0.5, time.Second, 0.8)), ShouldBeNil)
			So(influenceGraph.UpsertEdge(testEdge(EdgeInfluence, mediator, target, 0.6, time.Second, 0.9)), ShouldBeNil)

			paths := influenceGraph.Paths(source, target, 4)
			So(len(paths), ShouldEqual, 1)
			So(len(paths[0]), ShouldEqual, 2)
			So(paths[0][0].Result.Lag, ShouldEqual, time.Second)
		})
	})
}
