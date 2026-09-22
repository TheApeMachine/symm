package compiler

import (
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
)

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
