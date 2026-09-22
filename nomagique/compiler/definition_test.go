package compiler

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
A signal definition is only usable if an enclosing graph can reach what it
needs and read what it published. Compiling proves neither, so these exercise
the contract itself: a parent wires into the fields the sub-graph left open and
reads the fields nothing inside it consumed.
*/
func TestExpandDefinitionPorts(t *testing.T) {
	Convey("Given a graph enclosing a signal definition", t, func() {
		parent := Graph{
			ID: "parent",
			Nodes: map[string]Node{
				"feed": {
					ID:   "feed",
					Type: "arithmetic.Add",
					Connections: Connections{
						Outputs: map[string][]ConnectionTarget{
							"out": {{NodeID: "signal", PortName: "returns.value"}},
						},
					},
				},
				"signal": {
					ID:   "signal",
					Type: "definition:correlation_ticker",
					Connections: Connections{
						Inputs: map[string][]ConnectionTarget{
							"returns.value": {{NodeID: "feed", PortName: "out"}},
						},
						Outputs: map[string][]ConnectionTarget{
							"zscore.out": {{NodeID: "read", PortName: "value"}},
						},
					},
				},
				"read": {
					ID:   "read",
					Type: "statistic.Mean",
					Connections: Connections{
						Inputs: map[string][]ConnectionTarget{
							"value": {{NodeID: "signal", PortName: "zscore.out"}},
						},
					},
				},
			},
		}

		expanded, err := expandDefinitions(parent, DefaultRepository())
		So(err, ShouldBeNil)

		Convey("When the definition is expanded", func() {
			Convey("Then the definition node itself is gone", func() {
				_, present := expanded.Nodes["signal"]
				So(present, ShouldBeFalse)
			})

			Convey("Then an observation reaches the field the sub-graph left open", func() {
				returns, known := expanded.Nodes["signal__returns"]
				So(known, ShouldBeTrue)
				So(returns.Connections.Inputs["value"], ShouldResemble,
					[]ConnectionTarget{{NodeID: "feed", PortName: "out"}})
			})

			Convey("Then a published metric leaves for the enclosing graph", func() {
				zscore, known := expanded.Nodes["signal__zscore"]
				So(known, ShouldBeTrue)
				// The field also feeds the metric the signal publishes, so the
				// enclosing graph's wire is one of its consumers, not the only.
				So(zscore.Connections.Outputs["out"], ShouldContain,
					ConnectionTarget{NodeID: "read", PortName: "value"})
			})
		})

		Convey("When the enclosing graph is compiled", func() {
			program, err := Compile(parent, nil, DefaultRepository())
			So(err, ShouldBeNil)

			Convey("Then the observation and the metric are compiled routes", func() {
				feed, known := program.NodeMap["feed"]
				So(known, ShouldBeTrue)

				returns, known := program.NodeMap["signal__returns"]
				So(known, ShouldBeTrue)

				zscore, known := program.NodeMap["signal__zscore"]
				So(known, ShouldBeTrue)

				read, known := program.NodeMap["read"]
				So(known, ShouldBeTrue)

				var reached, published bool

				for _, route := range program.Routes {
					if route.FromNode == feed && route.ToNode == returns {
						reached = true
					}

					if route.FromNode == zscore && route.ToNode == read {
						published = true
					}
				}

				So(reached, ShouldBeTrue)
				So(published, ShouldBeTrue)
			})
		})
	})

	Convey("Given a parent wiring a field the definition does not have", t, func() {
		parent := Graph{
			ID: "parent",
			Nodes: map[string]Node{
				"signal": {
					ID:   "signal",
					Type: "definition:correlation_ticker",
					Connections: Connections{
						Inputs: map[string][]ConnectionTarget{
							"nowhere.value": {{NodeID: "feed", PortName: "out"}},
						},
					},
				},
			},
		}

		_, err := expandDefinitions(parent, DefaultRepository())

		Convey("Then the mistake is reported rather than silently dropped", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "nowhere")
		})
	})
}

/*
The system graph is where the signals meet the grid. Compiling it proves the
whole path: a signal's metrics are published from inside its sub-graph, cross
the definition boundary through the ports it left open, and arrive as the
capabilities the grid serves.
*/
func TestSystemGridServesPublishedMetrics(t *testing.T) {
	Convey("Given the system graph", t, func() {
		graph, err := DefaultRepository().Load("system")
		So(err, ShouldBeNil)

		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)

		Convey("When it is compiled", func() {
			index, known := program.NodeMap["grid"]
			So(known, ShouldBeTrue)

			grid := program.Nodes[index]
			So(grid.ArgsTemplate.IsValid(), ShouldBeTrue)

			Convey("Then every metric reaches it as a value of its own", func() {
				landing := 0

				for _, route := range program.Routes {
					if route.ToNode == index {
						landing++
					}
				}

				// Each metric is the value its last operation produced, wired
				// straight in. Nothing stands between the two.
				So(landing, ShouldBeGreaterThan, 400)
			})

			Convey("Then they came from inside the sub-graphs that computed them", func() {
				// A sub-graph groups a signal's metrics for the editor. It is
				// expanded away at compile time, so the operation that
				// produced a metric is a node like any other in the program.
				_, published := program.NodeMap["definition-correlation_ticker__zscore"]
				So(published, ShouldBeTrue)
			})
		})
	})
}
