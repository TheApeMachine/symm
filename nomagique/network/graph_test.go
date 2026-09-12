package network

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func intLess(left, right int) bool { return left < right }

// maxInt is the full-walk upper bound for the int-keyed test graph.
const maxInt = int(^uint(0) >> 1)

/*
command wraps one graph intent into the command wire and streams it through the
graph, returning the single result.
*/
func command(
	graph core.Primitive,
	cmd GraphCommand[int, string, string],
) (GraphResult[int, string, string], bool) {
	var result GraphResult[int, string, string]

	for ptr := range graph.Next(tests.SliceToSeq([]GraphCommand[int, string, string]{cmd})) {
		result = *(*GraphResult[int, string, string])(ptr)

		return result, true
	}

	return result, false
}

func TestGraphNext(t *testing.T) {
	Convey("Given an empty graph", t, func() {
		graph := NewGraph[int, string, string](intLess)

		Convey("it starts empty", func() {
			id := 1
			result, ok := command(graph, GraphCommand[int, string, string]{Outgoing: &id})

			So(ok, ShouldBeTrue)
			So(result.Outgoing, ShouldBeNil)

			count, ok := command(graph, GraphCommand[int, string, string]{Len: &Count{}})

			So(ok, ShouldBeTrue)
			So(count.Len, ShouldEqual, 0)
		})

		Convey("nodes can be set and updated in place", func() {
			_, ok := command(graph, GraphCommand[int, string, string]{
				SetNode: &Node[int, string]{ID: 1, Data: "one"},
			})
			So(ok, ShouldBeTrue)

			_, ok = command(graph, GraphCommand[int, string, string]{
				SetNode: &Node[int, string]{ID: 2, Data: "two"},
			})
			So(ok, ShouldBeTrue)

			id := 1
			result, ok := command(graph, GraphCommand[int, string, string]{Node: &id})
			So(ok, ShouldBeTrue)
			So(result.Found, ShouldBeTrue)
			So(result.Node.Data, ShouldEqual, "one")

			missing := 3
			result, ok = command(graph, GraphCommand[int, string, string]{Node: &missing})
			So(ok, ShouldBeTrue)
			So(result.Found, ShouldBeFalse)

			_, ok = command(graph, GraphCommand[int, string, string]{
				SetNode: &Node[int, string]{ID: 1, Data: "uno"},
			})
			So(ok, ShouldBeTrue)

			result, ok = command(graph, GraphCommand[int, string, string]{Node: &id})
			So(ok, ShouldBeTrue)
			So(result.Node.Data, ShouldEqual, "uno")
		})

		Convey("edges are directed and weighted", func() {
			_, ok := command(graph, GraphCommand[int, string, string]{
				SetEdge: &Edge[int, string]{From: 1, To: 2, Weight: 0.75, Data: "a"},
			})
			So(ok, ShouldBeTrue)

			Convey("outgoing is visible from the source only", func() {
				from, to := 1, 2

				result, ok := command(graph, GraphCommand[int, string, string]{Outgoing: &from})
				So(ok, ShouldBeTrue)
				So(result.Outgoing, ShouldHaveLength, 1)

				result, ok = command(graph, GraphCommand[int, string, string]{Outgoing: &to})
				So(ok, ShouldBeTrue)
				So(result.Outgoing, ShouldBeEmpty)
			})

			Convey("weight and sign are preserved", func() {
				id := 1

				result, ok := command(graph, GraphCommand[int, string, string]{Outgoing: &id})
				So(ok, ShouldBeTrue)
				So(result.Outgoing[0].Weight, ShouldEqual, 0.75)
				So(result.Outgoing[0].Data, ShouldEqual, "a")

				_, ok = command(graph, GraphCommand[int, string, string]{
					SetEdge: &Edge[int, string]{From: 1, To: 2, Weight: -1.5, Data: "b"},
				})
				So(ok, ShouldBeTrue)

				result, ok = command(graph, GraphCommand[int, string, string]{Outgoing: &id})
				So(ok, ShouldBeTrue)
				So(result.Outgoing[0].Weight, ShouldEqual, -1.5)
				So(result.Outgoing, ShouldHaveLength, 1)
			})

			Convey("reversing direction is a separate edge", func() {
				_, ok := command(graph, GraphCommand[int, string, string]{
					SetEdge: &Edge[int, string]{From: 2, To: 1, Weight: 0.25},
				})
				So(ok, ShouldBeTrue)

				from, to := 1, 2

				result, ok := command(graph, GraphCommand[int, string, string]{Outgoing: &from})
				So(ok, ShouldBeTrue)
				So(result.Outgoing, ShouldHaveLength, 1)

				result, ok = command(graph, GraphCommand[int, string, string]{Outgoing: &to})
				So(ok, ShouldBeTrue)
				So(result.Outgoing, ShouldHaveLength, 1)
			})
		})

		Convey("edges are updated in place without rebuilding the graph", func() {
			for _, edge := range []Edge[int, string]{
				{From: 1, To: 2, Weight: 1},
				{From: 1, To: 3, Weight: 2},
			} {
				_, ok := command(graph, GraphCommand[int, string, string]{SetEdge: &edge})
				So(ok, ShouldBeTrue)
			}

			id := 1

			result, ok := command(graph, GraphCommand[int, string, string]{Outgoing: &id})
			So(ok, ShouldBeTrue)
			So(result.Outgoing, ShouldHaveLength, 2)

			update := Edge[int, string]{From: 1, To: 2, Weight: 100}
			_, ok = command(graph, GraphCommand[int, string, string]{SetEdge: &update})
			So(ok, ShouldBeTrue)

			result, ok = command(graph, GraphCommand[int, string, string]{Outgoing: &id})
			So(ok, ShouldBeTrue)
			So(result.Outgoing, ShouldHaveLength, 2)

			walk, ok := command(graph, GraphCommand[int, string, string]{
				RangeEdges: &Span[int]{From: 0, To: maxInt},
			})
			So(ok, ShouldBeTrue)
			So(walk.Edges, ShouldHaveLength, 2)
		})

		Convey("range walks nodes in ascending key order", func() {
			for _, node := range []Node[int, string]{{ID: 2}, {ID: 1}, {ID: 3}} {
				_, ok := command(graph, GraphCommand[int, string, string]{SetNode: &node})
				So(ok, ShouldBeTrue)
			}

			result, ok := command(graph, GraphCommand[int, string, string]{
				RangeNodes: &Span[int]{From: 0, To: maxInt},
			})
			So(ok, ShouldBeTrue)

			ids := []int{}

			for _, node := range result.Nodes {
				ids = append(ids, node.ID)
			}

			So(ids, ShouldResemble, []int{1, 2, 3})
		})

		Convey("an ambiguous command ends the stream with an error", func() {
			id := 1
			cmd := GraphCommand[int, string, string]{
				Node: &id,
				Len:  &Count{},
			}

			count := 0

			for range graph.Next(tests.SliceToSeq([]GraphCommand[int, string, string]{cmd})) {
				count++
			}

			So(count, ShouldEqual, 0)
			So(graph.Error(), ShouldNotBeNil)
		})
	})
}

func TestGraphError(t *testing.T) {
	Convey("Given graph construction", t, func() {
		Convey("A nil ordering function is rejected", func() {
			graph := NewGraph[int, string, string](nil)

			So(graph.Error(), ShouldNotBeNil)

			count := 0

			for range graph.Next(tests.SliceToSeq([]GraphCommand[int, string, string]{})) {
				count++
			}

			So(count, ShouldEqual, 0)
		})

		Convey("A valid graph records no error", func() {
			graph := NewGraph[int, string, string](intLess)

			for range graph.Next(tests.SliceToSeq([]GraphCommand[int, string, string]{})) {
			}

			So(graph.Error(), ShouldBeNil)
		})
	})
}
