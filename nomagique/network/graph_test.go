package network

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func intLess(left, right int) bool { return left < right }

func TestGraph(t *testing.T) {
	Convey("Given an empty graph", t, func() {
		graph := NewGraph[int, string, string](intLess)

		Convey("it starts empty", func() {
			So(graph.Len(), ShouldEqual, 0)
			So(graph.Outgoing(1), ShouldBeNil)
		})

		Convey("nodes can be set and updated in place", func() {
			graph.SetNode(Node[int, string]{ID: 1, Data: "one"})
			graph.SetNode(Node[int, string]{ID: 2, Data: "two"})

			So(graph.HasNode(1), ShouldBeTrue)
			So(graph.HasNode(3), ShouldBeFalse)

			node, ok := graph.Node(1)
			So(ok, ShouldBeTrue)
			So(node.Data, ShouldEqual, "one")

			graph.SetNode(Node[int, string]{ID: 1, Data: "uno"})

			node, ok = graph.Node(1)
			So(ok, ShouldBeTrue)
			So(node.Data, ShouldEqual, "uno")
		})

		Convey("edges are directed and weighted", func() {
			graph.SetNode(Node[int, string]{ID: 1})
			graph.SetNode(Node[int, string]{ID: 2})

			graph.SetEdge(Edge[int, string]{From: 1, To: 2, Weight: 0.75, Data: "a"})

			Convey("outgoing is visible from the source only", func() {
				So(graph.Outgoing(1), ShouldHaveLength, 1)
				So(graph.Outgoing(2), ShouldBeEmpty)
			})

			Convey("weight and sign are preserved", func() {
				edge := graph.Outgoing(1)[0]
				So(edge.Weight, ShouldEqual, 0.75)
				So(edge.Data, ShouldEqual, "a")

				graph.SetEdge(Edge[int, string]{From: 1, To: 2, Weight: -1.5, Data: "b"})
				edge = graph.Outgoing(1)[0]
				So(edge.Weight, ShouldEqual, -1.5)
				So(len(graph.Outgoing(1)), ShouldEqual, 1)
			})

			Convey("reversing direction is a separate edge", func() {
				graph.SetEdge(Edge[int, string]{From: 2, To: 1, Weight: 0.25})
				So(graph.Outgoing(1), ShouldHaveLength, 1)
				So(graph.Outgoing(2), ShouldHaveLength, 1)
			})
		})

		Convey("edges are updated in place without rebuilding the graph", func() {
			graph.SetNode(Node[int, string]{ID: 1})
			graph.SetNode(Node[int, string]{ID: 2})
			graph.SetNode(Node[int, string]{ID: 3})

			graph.SetEdge(Edge[int, string]{From: 1, To: 2, Weight: 1})
			graph.SetEdge(Edge[int, string]{From: 1, To: 3, Weight: 2})

			So(graph.Outgoing(1), ShouldHaveLength, 2)

			graph.SetEdge(Edge[int, string]{From: 1, To: 2, Weight: 100})

			So(graph.Outgoing(1), ShouldHaveLength, 2)

			count := 0
			graph.RangeEdges(0, maxInt, func(Edge[int, string]) { count++ })
			So(count, ShouldEqual, 2)
		})

		Convey("range walks nodes in ascending key order", func() {
			graph.SetNode(Node[int, string]{ID: 2})
			graph.SetNode(Node[int, string]{ID: 1})
			graph.SetNode(Node[int, string]{ID: 3})

			var ids []int
			graph.Range(0, maxInt, func(node Node[int, string]) {
				ids = append(ids, node.ID)
			})

			So(ids, ShouldResemble, []int{1, 2, 3})
		})
	})
}

// maxInt is the full-walk upper bound for the int-keyed test graph.
const maxInt = int(^uint(0) >> 1)
