package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"testing"
	"time"

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

/* TestCompileSystemStages checks the shipping, explicitly wired LMAX topology. */
func TestCompileSystemStages(t *testing.T) {
	Convey("The shipping system owns metric execution through Cap'n Proto stages", t, func() {
		graph, err := DefaultRepository().Load("system")
		So(err, ShouldBeNil)
		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		So(graph.Nodes["workspace"].Type, ShouldEqual, "runtime.Workspace")
		So(len(graph.Nodes["metrics_group"].Connections.Inputs), ShouldEqual, 15)
		So(len(graph.Nodes["workspace"].Connections.Inputs), ShouldEqual, 10)
		_, flattened := program.NodeMap["signals__definition-correlation_ticker__zscore"]
		So(flattened, ShouldBeFalse)
		_, staged := program.NodeMap["definition-correlation_ticker_graph"]
		So(staged, ShouldBeTrue)
	})
}

/* callableWorkspace is a graph-authored pipeline of actual primitive definitions. */
func callableWorkspace() Graph {
	graph := Graph{ID: "callable", Nodes: map[string]Node{
		"read": {ID: "read", Type: "definition:read"},
		"mean": {ID: "mean", Type: "definition:mean"},
		"first": {ID: "first", Type: "runtime.Consumer", InputData: map[string]json.RawMessage{
			"name": json.RawMessage(`"market"`), "entry": json.RawMessage(`"extract.data"`), "outputs": json.RawMessage(`["extract"]`),
		}},
		"second": {ID: "second", Type: "runtime.Consumer", InputData: map[string]json.RawMessage{
			"name": json.RawMessage(`"average"`), "outputs": json.RawMessage(`["mean"]`),
			"bindings": json.RawMessage(`"[{\"producer\":\"market\",\"node\":\"extract\",\"field\":\"out\",\"target\":\"mean.value\"}]"`),
		}},
		"first-group":  {ID: "first-group", Type: "runtime.Group"},
		"second-group": {ID: "second-group", Type: "runtime.Group"},
		"workspace": {ID: "workspace", Type: "runtime.Workspace", InputData: map[string]json.RawMessage{
			"capacity": json.RawMessage(`8`), "writers": json.RawMessage(`1`), "epoch": json.RawMessage(`91`), "admit": json.RawMessage(`true`),
		}},
	}}

	for _, wire := range [][3]string{{"read", "first", "target"}, {"mean", "second", "target"}, {"first", "first-group", "consumers"}, {"second", "second-group", "consumers"}, {"first-group", "workspace", "groups_0"}, {"second-group", "workspace", "groups_1"}} {
		provider, consumer := graph.Nodes[wire[0]], graph.Nodes[wire[1]]

		if provider.Connections.Outputs == nil {
			provider.Connections.Outputs = map[string][]ConnectionTarget{}
		}

		if consumer.Connections.Inputs == nil {
			consumer.Connections.Inputs = map[string][]ConnectionTarget{}
		}
		provider.Connections.Outputs["self"] = append(provider.Connections.Outputs["self"], ConnectionTarget{NodeID: wire[1], PortName: wire[2]})
		consumer.Connections.Inputs[wire[2]] = append(consumer.Connections.Inputs[wire[2]], ConnectionTarget{NodeID: wire[0], PortName: "self"})
		graph.Nodes[wire[0]], graph.Nodes[wire[1]] = provider, consumer
	}
	return graph
}

func TestCompileDefinitionCapabilities(t *testing.T) {
	Convey("Given actual Extract and Mean definitions wired into LMAX groups", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		repository := NewRepository()
		So(repository.Save("read", []byte(`{"id":"read","nodes":{"extract":{"id":"extract","type":"data.Extract","inputData":{"path":"value"}}}}`)), ShouldBeNil)
		So(repository.Save("mean", []byte(`{"id":"mean","nodes":{"mean":{"id":"mean","type":"statistic.Mean"}}}`)), ShouldBeNil)
		program, err := Compile(callableWorkspace(), nil, repository)
		So(err, ShouldBeNil)
		defer program.Release()
		So(program.Execute(ctx, nil), ShouldBeNil)
		workspace := runtime.Workspace(program.Nodes[program.NodeMap["workspace"]].Client)

		for sequence := range 64 {
			So(workspace.Write(ctx, func(params runtime.Workspace_write_Params) error {
				payloads, err := params.NewData(1)

				if err != nil {
					return err
				}
				return payloads.Set(0, []byte(fmt.Sprintf(`{"value":%d}`, sequence+1)))
			}), ShouldBeNil)
			So(workspace.WaitStreaming(), ShouldBeNil)
		}
		flushed, release := workspace.Flush(ctx, nil)
		_, err = flushed.Struct()
		release()
		So(err, ShouldBeNil)
		consumer := runtime.Consumer(program.Nodes[program.NodeMap["second"]].Client)
		future, release := consumer.Done(ctx, nil)
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		So(result.Sequence(), ShouldEqual, 63)
		So(result.Completed(), ShouldEqual, 64)
		outputs, err := result.Outputs()
		So(err, ShouldBeNil)
		So(outputs.Len(), ShouldEqual, 1)
		So(outputs.At(0).Epoch(), ShouldEqual, 91)
		So(outputs.At(0).Sequence(), ShouldEqual, 63)
		pointer, err := outputs.At(0).Value()
		So(err, ShouldBeNil)
		So(statistic.Mean_done_Results(pointer.Struct()).Out(), ShouldEqual, 32.5)
	})
}

func TestCompileDefinitionCapabilitiesReplacement(t *testing.T) {
	Convey("A definition change replaces its immutable capability owners", t, func() {
		repository := NewRepository()
		So(repository.Save("read", []byte(`{"id":"read","nodes":{"extract":{"id":"extract","type":"data.Extract","inputData":{"path":"value"}}}}`)), ShouldBeNil)
		So(repository.Save("mean", []byte(`{"id":"mean","nodes":{"mean":{"id":"mean","type":"statistic.Mean"}}}`)), ShouldBeNil)
		original, err := Compile(callableWorkspace(), nil, repository)
		So(err, ShouldBeNil)
		defer original.Release()
		So(original.Execute(context.Background(), nil), ShouldBeNil)
		unchanged, err := CompileWithPrevious(callableWorkspace(), nil, original, repository)
		So(err, ShouldBeNil)
		defer unchanged.Release()

		for _, name := range []string{"read", "first", "first-group", "workspace"} {
			So(unchanged.Nodes[unchanged.NodeMap[name]].Client.IsSame(original.Nodes[original.NodeMap[name]].Client), ShouldBeTrue)
		}
		So(repository.Save("read", []byte(`{"id":"read","nodes":{"extract":{"id":"extract","type":"data.Extract","inputData":{"path":"replacement"}}}}`)), ShouldBeNil)
		replaced, err := CompileWithPrevious(callableWorkspace(), nil, original, repository)
		So(err, ShouldBeNil)
		defer replaced.Release()

		for _, name := range []string{"read", "first", "first-group", "workspace"} {
			So(replaced.Nodes[replaced.NodeMap[name]].Client.IsSame(original.Nodes[original.NodeMap[name]].Client), ShouldBeFalse)
		}
		So(replaced.Nodes[replaced.NodeMap["mean"]].Client.IsSame(original.Nodes[original.NodeMap["mean"]].Client), ShouldBeTrue)
		So(replaced.Execute(context.Background(), nil), ShouldBeNil)
	})
}
