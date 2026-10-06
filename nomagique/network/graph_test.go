package network_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/network"
)

func floatLess(left, right float64) bool { return left < right }

const maxFloat = math.MaxFloat64

func command(
	graph core.Primitive,
	intent string,
	numbers map[string]float64,
	texts map[string]string,
) (*data.Adapter, bool) {
	state := data.NewState(data.NewMap())
	adapter := data.NewAdapter(nil, state)

	if len(numbers) > 0 {
		payload := data.NewOutputMap()
		for key, value := range numbers {
			payload.Values[key] = value
		}
		for range adapter.Next(data.NewValue(payload)) {
		}
	}

	textPayload := data.NewTextMap()
	textPayload.Values["op"] = intent
	for key, value := range texts {
		textPayload.Values[key] = value
	}
	for range adapter.Next(data.NewValue(textPayload)) {
	}

	yielded := false
	for range graph.Next(data.NewValue(adapter)) {
		yielded = true
	}

	return adapter, yielded
}

func readNumbers(adapter *data.Adapter, keys ...string) data.Map[float64] {
	mapping := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		mapping = append(mapping, key, key)
	}
	request := data.NewMap(mapping...)
	var result data.Map[float64]
	for pointer := range adapter.Next(data.NewValue(request)) {
		result = *(*data.Map[float64])(pointer)
	}
	return result
}

func readTexts(adapter *data.Adapter, keys ...string) data.Map[string] {
	request := data.NewLiteral(keys...)
	var result data.Map[string]
	for pointer := range adapter.Next(data.NewValue(request)) {
		result = *(*data.Map[string])(pointer)
	}
	return result
}

func TestGraphNext(t *testing.T) {
	Convey("Given an empty graph", t, func() {
		graph := network.NewGraph(floatLess)

		Convey("it starts empty", func() {
			adapter, ok := command(graph, "outgoing", map[string]float64{"id": 1}, nil)
			So(ok, ShouldBeTrue)
			So(graph.Error(), ShouldBeNil)
			So(readNumbers(adapter, "count").Values["count"], ShouldEqual, 0)

			adapter, ok = command(graph, "len", nil, nil)
			So(ok, ShouldBeTrue)
			So(readNumbers(adapter, "len").Values["len"], ShouldEqual, 0)
		})

		Convey("nodes can be set and updated in place", func() {
			_, ok := command(graph, "set_node", map[string]float64{"id": 1}, map[string]string{"data": "one"})
			So(ok, ShouldBeTrue)

			_, ok = command(graph, "set_node", map[string]float64{"id": 2}, map[string]string{"data": "two"})
			So(ok, ShouldBeTrue)

			adapter, ok := command(graph, "get_node", map[string]float64{"id": 1}, nil)
			So(ok, ShouldBeTrue)
			So(readNumbers(adapter, "found").Values["found"], ShouldEqual, 1)
			So(readTexts(adapter, "data").Values["data"], ShouldEqual, "one")

			adapter, ok = command(graph, "get_node", map[string]float64{"id": 3}, nil)
			So(ok, ShouldBeTrue)
			So(readNumbers(adapter, "found").Values["found"], ShouldEqual, 0)

			_, ok = command(graph, "set_node", map[string]float64{"id": 1}, map[string]string{"data": "uno"})
			So(ok, ShouldBeTrue)

			adapter, ok = command(graph, "get_node", map[string]float64{"id": 1}, nil)
			So(ok, ShouldBeTrue)
			So(readTexts(adapter, "data").Values["data"], ShouldEqual, "uno")
		})

		Convey("edges are directed and weighted", func() {
			_, ok := command(graph, "set_edge",
				map[string]float64{"from": 1, "to": 2, "weight": 0.75},
				map[string]string{"data": "a"},
			)
			So(ok, ShouldBeTrue)

			Convey("outgoing is visible from the source only", func() {
				adapter, ok := command(graph, "outgoing", map[string]float64{"id": 1}, nil)
				So(ok, ShouldBeTrue)
				So(readNumbers(adapter, "count").Values["count"], ShouldEqual, 1)

				adapter, ok = command(graph, "outgoing", map[string]float64{"id": 2}, nil)
				So(ok, ShouldBeTrue)
				So(readNumbers(adapter, "count").Values["count"], ShouldEqual, 0)
			})

			Convey("weight and sign are preserved", func() {
				adapter, ok := command(graph, "outgoing", map[string]float64{"id": 1}, nil)
				So(ok, ShouldBeTrue)
				numbers := readNumbers(adapter, "edge.0.weight")
				So(numbers.Values["edge.0.weight"], ShouldEqual, 0.75)
				So(readTexts(adapter, "edge.0.data").Values["edge.0.data"], ShouldEqual, "a")

				_, ok = command(graph, "set_edge",
					map[string]float64{"from": 1, "to": 2, "weight": -1.5},
					map[string]string{"data": "b"},
				)
				So(ok, ShouldBeTrue)

				adapter, ok = command(graph, "outgoing", map[string]float64{"id": 1}, nil)
				So(ok, ShouldBeTrue)
				So(readNumbers(adapter, "count", "edge.0.weight").Values["count"], ShouldEqual, 1)
				So(readNumbers(adapter, "edge.0.weight").Values["edge.0.weight"], ShouldEqual, -1.5)
			})

			Convey("reversing direction is a separate edge", func() {
				_, ok := command(graph, "set_edge",
					map[string]float64{"from": 2, "to": 1, "weight": 0.25},
					nil,
				)
				So(ok, ShouldBeTrue)

				adapter, ok := command(graph, "outgoing", map[string]float64{"id": 1}, nil)
				So(ok, ShouldBeTrue)
				So(readNumbers(adapter, "count").Values["count"], ShouldEqual, 1)

				adapter, ok = command(graph, "outgoing", map[string]float64{"id": 2}, nil)
				So(ok, ShouldBeTrue)
				So(readNumbers(adapter, "count").Values["count"], ShouldEqual, 1)
			})
		})

		Convey("edges are updated in place without rebuilding the graph", func() {
			for _, edge := range [][3]float64{{1, 2, 1}, {1, 3, 2}} {
				_, ok := command(graph, "set_edge",
					map[string]float64{"from": edge[0], "to": edge[1], "weight": edge[2]},
					nil,
				)
				So(ok, ShouldBeTrue)
			}

			adapter, ok := command(graph, "outgoing", map[string]float64{"id": 1}, nil)
			So(ok, ShouldBeTrue)
			So(readNumbers(adapter, "count").Values["count"], ShouldEqual, 2)

			_, ok = command(graph, "set_edge",
				map[string]float64{"from": 1, "to": 2, "weight": 100},
				nil,
			)
			So(ok, ShouldBeTrue)

			adapter, ok = command(graph, "outgoing", map[string]float64{"id": 1}, nil)
			So(ok, ShouldBeTrue)
			So(readNumbers(adapter, "count").Values["count"], ShouldEqual, 2)

			adapter, ok = command(graph, "range_edges",
				map[string]float64{"from": 0, "to": maxFloat},
				nil,
			)
			So(ok, ShouldBeTrue)
			So(readNumbers(adapter, "count").Values["count"], ShouldEqual, 2)
		})

		Convey("range walks nodes in ascending key order", func() {
			for _, id := range []float64{2, 1, 3} {
				_, ok := command(graph, "set_node", map[string]float64{"id": id}, nil)
				So(ok, ShouldBeTrue)
			}

			adapter, ok := command(graph, "range_nodes",
				map[string]float64{"from": 0, "to": maxFloat},
				nil,
			)
			So(ok, ShouldBeTrue)
			numbers := readNumbers(adapter, "count", "node.0.id", "node.1.id", "node.2.id")
			So(numbers.Values["count"], ShouldEqual, 3)
			So(numbers.Values["node.0.id"], ShouldEqual, 1)
			So(numbers.Values["node.1.id"], ShouldEqual, 2)
			So(numbers.Values["node.2.id"], ShouldEqual, 3)
		})

		Convey("an unknown op ends the stream with an error", func() {
			count := 0
			_, ok := command(graph, "nope", nil, nil)
			So(ok, ShouldBeFalse)
			So(count, ShouldEqual, 0)
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
			for range graph.Next(data.NewValue((*data.Adapter)(nil))) {
				count++
			}
			So(count, ShouldEqual, 0)
		})
	})
}
