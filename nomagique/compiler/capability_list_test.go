package compiler

import (
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
)

/*
A grid is as wide as the graph made it: every metric wired into its capability
list port is kept, rather than replacing the one wired before it.
*/
func TestBindCapabilityList(t *testing.T) {
	Convey("Given a grid with several metrics wired into it", t, func() {
		graph := Graph{
			ID: "grid-fixture",
			Nodes: map[string]Node{
				"first": {
					ID:   "first",
					Type: "data.MetricService",
					Connections: Connections{
						Outputs: map[string][]ConnectionTarget{
							"read": {{NodeID: "grid", PortName: "metrics"}},
						},
					},
				},
				"second": {
					ID:   "second",
					Type: "data.MetricService",
					Connections: Connections{
						Outputs: map[string][]ConnectionTarget{
							"read": {{NodeID: "grid", PortName: "metrics_1"}},
						},
					},
				},
				"third": {
					ID:   "third",
					Type: "data.MetricService",
					Connections: Connections{
						Outputs: map[string][]ConnectionTarget{
							"read": {{NodeID: "grid", PortName: "metrics_2"}},
						},
					},
				},
				"grid": {
					ID:   "grid",
					Type: "store.Grid",
					Connections: Connections{
						Inputs: map[string][]ConnectionTarget{
							"metrics":   {{NodeID: "first", PortName: "read"}},
							"metrics_1": {{NodeID: "second", PortName: "read"}},
							"metrics_2": {{NodeID: "third", PortName: "read"}},
						},
					},
				},
			},
		}

		program, err := Compile(graph, nil, nil)
		So(err, ShouldBeNil)
		So(program, ShouldNotBeNil)

		Convey("When the program is compiled", func() {
			index, known := program.NodeMap["grid"]
			So(known, ShouldBeTrue)

			grid := program.Nodes[index]
			So(grid.ArgsTemplate.IsValid(), ShouldBeTrue)

			schema, err := ReflectInterface(grid.Write.InterfaceID)
			So(err, ShouldBeNil)

			field, resolved := resolveInputField(schema, "metrics")
			So(resolved, ShouldBeTrue)

			Convey("Then the port is recognised as carrying a list of capabilities", func() {
				So(field.CapabilityList, ShouldBeTrue)
			})

			Convey("Then every wired metric is kept, not just the last one", func() {
				pointer, err := grid.ArgsTemplate.Ptr(uint16(field.Offset))
				So(err, ShouldBeNil)
				So(pointer.IsValid(), ShouldBeTrue)
				So(pointer.List().Len(), ShouldEqual, 3)
			})
		})
	})
}

/*
A capability port that carries one capability keeps binding exactly one, so
adding list support did not turn every interface port into a list.
*/
func TestBindCapabilitySingle(t *testing.T) {
	Convey("Given a primitive whose capability port carries one capability", t, func() {
		Convey("When the grid schema is reflected", func() {
			schema, err := ReflectInterface(gridInterfaceID(t))
			So(err, ShouldBeNil)

			Convey("Then only the metrics port is a capability list", func() {
				So(schema.Inputs["metrics"].CapabilityList, ShouldBeTrue)
				So(schema.Inputs["data"].CapabilityList, ShouldBeFalse)
				So(schema.Inputs["interests"].CapabilityList, ShouldBeFalse)
			})
		})
	})
}

func gridInterfaceID(t *testing.T) uint64 {
	t.Helper()

	factory, err := DefaultRegistry().Resolve("store.Grid")
	So(err, ShouldBeNil)

	return factory.InterfaceID
}

/*
A gathering port keeps every producer that lands on it. Without this, several
feeds wired into one input would resolve to the same slot and the last writer
would be the only one the node ever saw.
*/
func TestCompileFanIn(t *testing.T) {
	Convey("Given several feeds landing on one grid data port", t, func() {
		feed := func(id, port string) Node {
			return Node{
				ID:   id,
				Type: "websocket.WebSocketClient",
				Connections: Connections{
					Outputs: map[string][]ConnectionTarget{
						"read": {{NodeID: "grid", PortName: port}},
					},
				},
			}
		}

		graph := Graph{
			ID: "fan-in-fixture",
			Nodes: map[string]Node{
				"first":  feed("first", "data"),
				"second": feed("second", "data_1"),
				"third":  feed("third", "data_2"),
				"grid": {
					ID:   "grid",
					Type: "store.Grid",
					Connections: Connections{
						Inputs: map[string][]ConnectionTarget{
							"data":   {{NodeID: "first", PortName: "read"}},
							"data_1": {{NodeID: "second", PortName: "read"}},
							"data_2": {{NodeID: "third", PortName: "read"}},
						},
					},
				},
			},
		}

		program, err := Compile(graph, nil, nil)
		So(err, ShouldBeNil)

		index, known := program.NodeMap["grid"]
		So(known, ShouldBeTrue)

		grid := program.Nodes[index]

		schema, err := ReflectInterface(grid.Write.InterfaceID)
		So(err, ShouldBeNil)

		field, resolved := resolveInputField(schema, "data")
		So(resolved, ShouldBeTrue)

		Convey("When the graph is compiled", func() {
			Convey("Then the port is recognised as gathering values", func() {
				So(field.ValueList, ShouldBeTrue)
				So(field.CapabilityList, ShouldBeFalse)
			})

			Convey("Then every feed gets a route of its own", func() {
				landing := 0

				for _, route := range program.Routes {
					if route.ToNode == index {
						landing++
					}
				}

				So(landing, ShouldEqual, 3)
			})

			Convey("Then every feed keeps a slot of its own", func() {
				_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
				So(err, ShouldBeNil)

				carried, err := capnp.NewStruct(segment, capnp.ObjectSize{PointerCount: 4})
				So(err, ShouldBeNil)

				landing := 0

				for _, route := range program.Routes {
					if route.ToNode != index {
						continue
					}

					So(carried.SetNewText(0, "payload"), ShouldBeNil)
					So(route.Copy(carried, grid.ArgsTemplate), ShouldBeNil)
					landing++
				}

				pointer, err := grid.ArgsTemplate.Ptr(uint16(field.Offset))
				So(err, ShouldBeNil)
				So(pointer.IsValid(), ShouldBeTrue)

				// Three feeds landed and all three are still there: a
				// gathering port accumulates rather than overwrites.
				So(landing, ShouldEqual, 3)
				So(pointer.List().Len(), ShouldEqual, 3)
			})
		})
	})
}
