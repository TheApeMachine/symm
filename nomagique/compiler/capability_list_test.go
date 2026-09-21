package compiler

import (
	"testing"

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
