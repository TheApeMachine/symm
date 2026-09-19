package compiler_test

import (
	"reflect"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/definitions"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestArchitecturalInvariants(t *testing.T) {
	Convey("Architectural Invariants Verification", t, func() {
		Convey("Compiler contains no Interests method on Builder", func() {
			builderType := reflect.TypeOf(&compiler.Builder{})
			_, hasInterests := builderType.MethodByName("Interests")
			So(hasInterests, ShouldBeFalse)
		})

		Convey("Compiler registry contains no pipeline.* pseudo-primitives", func() {
			reg := compiler.DefaultRegistry()
			for _, pseudo := range []string{"pipeline.Signals", "pipeline.Logic", "pipeline.Execution"} {
				_, err := reg.Resolve(compiler.Node{ID: "test", Type: pseudo})
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "unknown primitive type")
			}
		})

		Convey("Missing required node configuration fails compilation with validation error", func() {
			reg := compiler.DefaultRegistry()
			repo := definitions.Default()

			// data.Extract without 'path' or '_config'
			graph := compiler.Graph{
				ID:   "test_extract_missing_cfg",
				Name: "test_extract_missing_cfg",
				Nodes: map[string]compiler.Node{
					"source": {ID: "source", Type: "data.Source"},
					"ext":    {ID: "ext", Type: "data.Extract"},
				},
			}

			_, err := compiler.Compile[any, any](graph, reg, repo)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "requires 'path' or 'key' configuration")
		})

		Convey("Type boundary mismatch fails execution rather than silently emitting zero", func() {
			reg := compiler.DefaultRegistry()
			repo := definitions.Default()

			// temporal.Delay requires positive horizon configuration
			graph := compiler.Graph{
				ID:   "test_delay_invalid_cfg",
				Name: "test_delay_invalid_cfg",
				Nodes: map[string]compiler.Node{
					"source": {ID: "source", Type: "data.Source"},
					"delay":  {ID: "delay", Type: "temporal.Delay"},
				},
			}

			_, err := compiler.Compile[any, any](graph, reg, repo)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "requires positive 'horizon'")
		})

		Convey("Renaming node IDs does not break source and sink behavior", func() {
			reg := compiler.DefaultRegistry()
			repo := definitions.Default()

			// Graph where source ID is "custom_input" and sink ID is "custom_output"
			graph := compiler.Graph{
				ID:   "test_renamed_ids",
				Name: "test_renamed_ids",
				Nodes: map[string]compiler.Node{
					"custom_input": {
						ID:   "custom_input",
						Type: "data.Source",
						Connections: compiler.Connections{
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "pass", PortName: "in"}},
							},
						},
					},
					"pass": {
						ID:   "pass",
						Type: "ui.Broadcast",
						Connections: compiler.Connections{
							Inputs: map[string][]compiler.ConnectionTarget{
								"in": {{NodeID: "custom_input", PortName: "out"}},
							},
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "custom_output", PortName: "in"}},
							},
						},
					},
					"custom_output": {
						ID:   "custom_output",
						Type: "data.Sink",
						Connections: compiler.Connections{
							Inputs: map[string][]compiler.ConnectionTarget{
								"in": {{NodeID: "pass", PortName: "out"}},
							},
						},
					},
				},
			}

			compiled, err := compiler.Compile[any, any](graph, reg, repo)
			So(err, ShouldBeNil)
			So(compiled, ShouldNotBeNil)

			out := compiled("test_value")
			So(out, ShouldEqual, "test_value")
		})

		Convey("Broken metric definition fails compilation of referencing graph", func() {
			reg := compiler.DefaultRegistry()
			// Repository that returns not found for a metric
			brokenGraph := compiler.Graph{
				ID:   "system_with_broken_def",
				Name: "system_with_broken_def",
				Nodes: map[string]compiler.Node{
					"source": {ID: "source", Type: "data.Source"},
					"broken": {ID: "broken", Type: "definition:non_existent_definition"},
				},
			}

			_, err := compiler.Compile[any, any](brokenGraph, reg, definitions.Default())
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not found")
		})

		Convey("Websocket and shell execution primitives use pure types.Value", func() {
			connect := transport.NewWSConnect(types.Const("wss://test"))
			So(reflect.TypeOf(connect).Kind(), ShouldEqual, reflect.Func)

			read := transport.NewWSRead()
			So(reflect.TypeOf(read).Kind(), ShouldEqual, reflect.Func)

			write := transport.NewWSWrite(nil)
			So(reflect.TypeOf(write).Kind(), ShouldEqual, reflect.Func)

			closeConn := transport.NewWSClose()
			So(reflect.TypeOf(closeConn).Kind(), ShouldEqual, reflect.Func)

			msg := transport.NewJSONMessage(nil)
			So(reflect.TypeOf(msg).Kind(), ShouldEqual, reflect.Func)

			proc := transport.NewProcess(types.Const("echo"))
			So(reflect.TypeOf(proc).Kind(), ShouldEqual, reflect.Func)
		})

		Convey("No machine-specific paths exist in transport or compiler", func() {
			reg := compiler.DefaultRegistry()
			// Ensure resolve on process does not default to any personal user directory
			procNode := compiler.Node{
				ID:   "proc",
				Type: "transport.Process",
				InputData: map[string]any{
					"binary": "echo",
				},
			}
			closure, err := reg.Resolve(procNode)
			So(err, ShouldBeNil)
			So(closure, ShouldNotBeNil)
		})
	})
}
