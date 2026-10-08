package network_test

import (
	"math"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/network"
)

func floatLess(left, right float64) bool { return left < right }

const maxFloat = math.MaxFloat64

func execCommand(graph core.Primitive, cmd *network.Command) (*network.Result, bool) {
	var result *network.Result
	yielded := false

	for ptr := range graph.Next(data.NewValue(unsafe.Pointer(cmd)).Next(nil)) {
		result = (*network.Result)(ptr)
		yielded = true
	}

	return result, yielded
}

func TestGraphNext(t *testing.T) {
	Convey("Given an empty graph", t, func() {
		graph := network.NewGraph(floatLess)

		Convey("it starts empty", func() {
			res, ok := execCommand(graph, &network.Command{Op: "outgoing", ID: 1})
			So(ok, ShouldBeTrue)
			So(graph.Error(), ShouldBeNil)
			So(res.Count, ShouldEqual, 0)

			res, ok = execCommand(graph, &network.Command{Op: "len"})
			So(ok, ShouldBeTrue)
			So(res.Len, ShouldEqual, 0)
		})

		Convey("nodes can be set and updated in place", func() {
			_, ok := execCommand(graph, &network.Command{Op: "set_node", ID: 1, Data: "one"})
			So(ok, ShouldBeTrue)

			_, ok = execCommand(graph, &network.Command{Op: "set_node", ID: 2, Data: "two"})
			So(ok, ShouldBeTrue)

			res, ok := execCommand(graph, &network.Command{Op: "get_node", ID: 1})
			So(ok, ShouldBeTrue)
			So(res.Found, ShouldEqual, 1)
			So(res.Data, ShouldEqual, "one")

			res, ok = execCommand(graph, &network.Command{Op: "get_node", ID: 3})
			So(ok, ShouldBeTrue)
			So(res.Found, ShouldEqual, 0)

			_, ok = execCommand(graph, &network.Command{Op: "set_node", ID: 1, Data: "uno"})
			So(ok, ShouldBeTrue)

			res, ok = execCommand(graph, &network.Command{Op: "get_node", ID: 1})
			So(ok, ShouldBeTrue)
			So(res.Data, ShouldEqual, "uno")
		})

		Convey("edges are directed and weighted", func() {
			_, ok := execCommand(graph, &network.Command{
				Op:     "set_edge",
				From:   1,
				To:     2,
				Weight: 0.75,
				Data:   "a",
			})
			So(ok, ShouldBeTrue)

			Convey("outgoing is visible from the source only", func() {
				res, ok := execCommand(graph, &network.Command{Op: "outgoing", ID: 1})
				So(ok, ShouldBeTrue)
				So(res.Count, ShouldEqual, 1)

				res, ok = execCommand(graph, &network.Command{Op: "outgoing", ID: 2})
				So(ok, ShouldBeTrue)
				So(res.Count, ShouldEqual, 0)
			})

			Convey("weight and sign are preserved", func() {
				res, ok := execCommand(graph, &network.Command{Op: "outgoing", ID: 1})
				So(ok, ShouldBeTrue)
				So(len(res.Edges), ShouldEqual, 1)
				So(res.Edges[0].Weight, ShouldEqual, 0.75)
				So(res.Edges[0].Data, ShouldEqual, "a")

				_, ok = execCommand(graph, &network.Command{
					Op:     "set_edge",
					From:   1,
					To:     2,
					Weight: -1.5,
					Data:   "b",
				})
				So(ok, ShouldBeTrue)

				res, ok = execCommand(graph, &network.Command{Op: "outgoing", ID: 1})
				So(ok, ShouldBeTrue)
				So(res.Count, ShouldEqual, 1)
				So(res.Edges[0].Weight, ShouldEqual, -1.5)
			})

			Convey("reversing direction is a separate edge", func() {
				_, ok := execCommand(graph, &network.Command{
					Op:     "set_edge",
					From:   2,
					To:     1,
					Weight: 0.25,
				})
				So(ok, ShouldBeTrue)

				res, ok := execCommand(graph, &network.Command{Op: "outgoing", ID: 1})
				So(ok, ShouldBeTrue)
				So(res.Count, ShouldEqual, 1)

				res, ok = execCommand(graph, &network.Command{Op: "outgoing", ID: 2})
				So(ok, ShouldBeTrue)
				So(res.Count, ShouldEqual, 1)
			})
		})

		Convey("edges are updated in place without rebuilding the graph", func() {
			for _, edge := range [][3]float64{{1, 2, 1}, {1, 3, 2}} {
				_, ok := execCommand(graph, &network.Command{
					Op:     "set_edge",
					From:   edge[0],
					To:     edge[1],
					Weight: edge[2],
				})
				So(ok, ShouldBeTrue)
			}

			res, ok := execCommand(graph, &network.Command{Op: "outgoing", ID: 1})
			So(ok, ShouldBeTrue)
			So(res.Count, ShouldEqual, 2)

			_, ok = execCommand(graph, &network.Command{
				Op:     "set_edge",
				From:   1,
				To:     2,
				Weight: 100,
			})
			So(ok, ShouldBeTrue)

			res, ok = execCommand(graph, &network.Command{Op: "outgoing", ID: 1})
			So(ok, ShouldBeTrue)
			So(res.Count, ShouldEqual, 2)

			res, ok = execCommand(graph, &network.Command{
				Op:   "range_edges",
				From: 0,
				To:   maxFloat,
			})
			So(ok, ShouldBeTrue)
			So(res.Count, ShouldEqual, 2)
		})

		Convey("range walks nodes in ascending key order", func() {
			for _, id := range []float64{2, 1, 3} {
				_, ok := execCommand(graph, &network.Command{Op: "set_node", ID: id})
				So(ok, ShouldBeTrue)
			}

			res, ok := execCommand(graph, &network.Command{
				Op:   "range_nodes",
				From: 0,
				To:   maxFloat,
			})
			So(ok, ShouldBeTrue)
			So(res.Count, ShouldEqual, 3)
			So(res.Nodes[0].ID, ShouldEqual, 1)
			So(res.Nodes[1].ID, ShouldEqual, 2)
			So(res.Nodes[2].ID, ShouldEqual, 3)
		})

		Convey("an unknown op ends the stream with an error", func() {
			_, ok := execCommand(graph, &network.Command{Op: "nope"})
			So(ok, ShouldBeFalse)
			So(graph.Error(), ShouldNotBeNil)
		})
	})
}

func TestGraphError(t *testing.T) {
	Convey("Given graph construction", t, func() {
		Convey("A nil ordering function is rejected", func() {
			graph := network.NewGraph(nil)
			So(graph.Error(), ShouldNotBeNil)

			count := 0
			dummyCmd := &network.Command{Op: "len"}

			for range graph.Next(data.NewValue(unsafe.Pointer(dummyCmd)).Next(nil)) {
				count++
			}

			So(count, ShouldEqual, 0)
		})
	})
}
