package compiler

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func capabilityGraph(bodyProvider string) Graph {
	return Graph{
		ID:   "capability",
		Name: "capability",
		Nodes: map[string]Node{
			"body": {
				ID:   "body",
				Type: bodyProvider,
				Connections: Connections{
					Inputs: map[string][]ConnectionTarget{},
					Outputs: map[string][]ConnectionTarget{
						"self": {{NodeID: "mapper", PortName: "body"}},
					},
				},
			},
			"mapper": {
				ID:        "mapper",
				Type:      "data.Map",
				InputData: map[string]json.RawMessage{"path": json.RawMessage(`"sizes"`)},
				Connections: Connections{
					Inputs: map[string][]ConnectionTarget{},
					Outputs: map[string][]ConnectionTarget{
						"out": {{NodeID: "sink", PortName: "value"}},
					},
				},
			},
			"sink": {
				ID:   "sink",
				Type: "sink",
				Connections: Connections{
					Inputs:  map[string][]ConnectionTarget{},
					Outputs: map[string][]ConnectionTarget{},
				},
			},
		},
	}
}

func TestCapabilityEdges(t *testing.T) {
	Convey("Given a graph wiring a node into a capability port", t, func() {
		Convey("It binds a provider implementing the required interface", func() {
			program, err := Compile(
				capabilityGraph("data.Scale"), nil, DefaultRepository(),
			)
			So(err, ShouldBeNil)
			So(len(program.Nodes), ShouldEqual, 3)

			Convey("The capability is bound rather than routed as a value", func() {
				for _, route := range program.Routes {
					So(
						program.Nodes[route.ToNode].ID+"."+
							program.Nodes[route.ToNode].Inputs["body"].Name,
						ShouldNotEqual,
						"mapper.body",
					)
				}
			})

			Convey("The consumer carries the capability on every call", func() {
				mapper := program.Nodes[program.NodeMap["mapper"]]
				So(mapper.ArgsTemplate.IsValid(), ShouldBeTrue)

				pointer, err := mapper.ArgsTemplate.Ptr(
					uint16(mapper.Inputs["body"].Offset),
				)
				So(err, ShouldBeNil)
				So(pointer.Interface().IsValid(), ShouldBeTrue)
			})
		})

		Convey("It refuses a provider that does not implement the interface", func() {
			_, err := Compile(
				capabilityGraph("calculus.Square"), nil, DefaultRepository(),
			)
			So(err, ShouldNotBeNil)
		})
	})
}
