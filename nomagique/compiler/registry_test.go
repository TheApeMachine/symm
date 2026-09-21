package compiler_test

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
)

func TestCatalogAndRegistryConsistency(t *testing.T) {
	Convey("Given the default compiler registry", t, func() {
		reg := compiler.DefaultRegistry()
		So(reg, ShouldNotBeNil)

		Convey("Core computational primitives must resolve in compiler registry", func() {
			coreOps := []string{
				"arithmetic.Add",
				"arithmetic.Subtract",
				"arithmetic.Multiply",
				"arithmetic.Divide",
				"calculus.Square",
				"calculus.Floor",
				"data.Source",
			}

			for _, op := range coreOps {
				So(reg.Has(op), ShouldBeTrue)
			}
		})
	})
}

func TestWorkbenchCompileAndRun(t *testing.T) {
	Convey("Given a workbench runner and a simple graph: Add(2, 3) -> Square", t, func() {
		runner := compiler.NewWorkbenchRunner()
		So(runner, ShouldNotBeNil)

		// Construct graph:
		// node1: arithmetic.Add, inputs a=2, b=3
		// node2: calculus.Square, input in connected to node1.out
		testGraph := map[string]any{
			"nodes": map[string]any{
				"add_1": map[string]any{
					"id":   "add_1",
					"type": "arithmetic.Add",
					"inputData": map[string]any{
						"a": map[string]any{"value": 2.0},
						"b": map[string]any{"value": 3.0},
					},
					"connections": map[string]any{
						"inputs": map[string]any{},
						"outputs": map[string]any{
							"out": []map[string]any{
								{"nodeId": "sq_1", "portName": "value"},
							},
						},
					},
				},
				"sq_1": map[string]any{
					"id":   "sq_1",
					"type": "calculus.Square",
					"inputData": map[string]any{},
					"connections": map[string]any{
						"inputs": map[string]any{
							"value": []map[string]any{
								{"nodeId": "add_1", "portName": "out"},
							},
						},
						"outputs": map[string]any{},
					},
				},
			},
		}

		rawBytes, err := json.Marshal(testGraph)
		So(err, ShouldBeNil)

		Convey("When compiling the graph", func() {
			res, err := runner.Compile(rawBytes)
			So(err, ShouldBeNil)
			compileResp, ok := res.(compiler.CompileResponse)
			So(ok, ShouldBeTrue)
			So(compileResp.OK, ShouldBeTrue)
			So(compileResp.NodeCount, ShouldEqual, 2)
			So(compileResp.RouteCount, ShouldEqual, 1)
		})

		Convey("When executing the graph via Run", func() {
			res, err := runner.Run(context.Background(), rawBytes)
			So(err, ShouldBeNil)
			runResp, ok := res.(compiler.RunResponse)
			So(ok, ShouldBeTrue)
			So(runResp.OK, ShouldBeTrue)
			So(runResp.Results, ShouldNotBeNil)

			// add_1 should produce 5.0
			addRes, ok := runResp.Results["add_1"]
			So(ok, ShouldBeTrue)
			So(addRes["out"], ShouldEqual, 5.0)

			// sq_1 should produce 25.0
			sqRes, ok := runResp.Results["sq_1"]
			So(ok, ShouldBeTrue)
			So(sqRes["out"], ShouldEqual, 25.0)
		})

		Convey("When compiling a graph with types.JSON and websocket.WebSocketClient", func() {
			jsonGraph := map[string]any{
				"nodes": map[string]any{
					"json_1": map[string]any{
						"id":   "json_1",
						"type": "types.JSON",
						"inputData": map[string]any{
							"text": map[string]any{"text": `{"method":"subscribe"}`},
						},
					},
					"ws_1": map[string]any{
						"id":   "ws_1",
						"type": "websocket.WebSocketClient",
						"inputData": map[string]any{
							"endpoint": map[string]any{"text": "wss://ws.kraken.com/v2"},
						},
					},
				},
			}
			jsonBytes, err := json.Marshal(jsonGraph)
			So(err, ShouldBeNil)
			res, err := runner.Compile(jsonBytes)
			So(err, ShouldBeNil)
			compileResp, ok := res.(compiler.CompileResponse)
			So(ok, ShouldBeTrue)
			So(compileResp.OK, ShouldBeTrue)
		})

		Convey("When compiling an invalid graph with a type mismatch", func() {
			badGraph := map[string]any{
				"nodes": map[string]any{
					"add_1": map[string]any{
						"id":   "add_1",
						"type": "arithmetic.Add",
						"connections": map[string]any{
							"outputs": map[string]any{
								"out": []map[string]any{
									{"nodeId": "class_1", "portName": "class"},
								},
							},
						},
					},
					"class_1": map[string]any{
						"id":   "class_1",
						"type": "cognition.Classification",
						"connections": map[string]any{
							"inputs": map[string]any{
								"class": []map[string]any{
									{"nodeId": "add_1", "portName": "out"},
								},
							},
						},
					},
				},
			}

			badBytes, _ := json.Marshal(badGraph)
			res, err := runner.Compile(badBytes)
			So(err, ShouldNotBeNil)
			compileResp, ok := res.(compiler.CompileResponse)
			So(ok, ShouldBeTrue)
			So(compileResp.OK, ShouldBeFalse)
			So(len(compileResp.Diagnostics), ShouldBeGreaterThan, 0)
			So(compileResp.Diagnostics[0].Kind, ShouldEqual, "incompatible_edge")
		})
	})
}

func TestWorkbenchNestedDefinitionExpansion(t *testing.T) {
	Convey("Given an inner definition saved to signal: inner = Add(2, 3)", t, func() {
		innerDef := map[string]any{
			"nodes": map[string]any{
				"add_node": map[string]any{
					"id":   "add_node",
					"type": "arithmetic.Add",
					"inputData": map[string]any{
						"a": map[string]any{"value": 2.0},
						"b": map[string]any{"value": 3.0},
					},
					"connections": map[string]any{
						"outputs": map[string]any{
							"out": []map[string]any{
								{"nodeId": "sink", "portName": "in"},
							},
						},
					},
				},
				"sink": map[string]any{
					"id":   "sink",
					"type": "data.Sink",
					"connections": map[string]any{
						"inputs": map[string]any{
							"in": []map[string]any{
								{"nodeId": "add_node", "portName": "out"},
							},
						},
					},
				},
			},
		}
		innerBytes, err := json.Marshal(innerDef)
		So(err, ShouldBeNil)

		err = compiler.DefaultRepository().Save("inner", innerBytes)
		So(err, ShouldBeNil)

		compiler.SetDefaultDefinitionRepository(compiler.DefaultRepository())
		runner := compiler.NewWorkbenchRunner()

		Convey("When outer graph references definition:inner -> calculus.Square", func() {
			outerDef := map[string]any{
				"nodes": map[string]any{
					"inner_node": map[string]any{
						"id":   "inner_node",
						"type": "definition:inner",
						"connections": map[string]any{
							"outputs": map[string]any{
								"out": []map[string]any{
									{"nodeId": "sq_node", "portName": "value"},
								},
							},
						},
					},
					"sq_node": map[string]any{
						"id":   "sq_node",
						"type": "calculus.Square",
						"connections": map[string]any{
							"inputs": map[string]any{
								"value": []map[string]any{
									{"nodeId": "inner_node", "portName": "out"},
								},
							},
						},
					},
				},
			}
			outerBytes, err := json.Marshal(outerDef)
			So(err, ShouldBeNil)

			runRes, err := runner.Run(context.Background(), outerBytes)
			So(err, ShouldBeNil)
			resp, ok := runRes.(compiler.RunResponse)
			So(ok, ShouldBeTrue)
			So(resp.OK, ShouldBeTrue)

			// sq_node should receive 5.0 from expanded inner definition and produce 25.0!
			sqRes, ok := resp.Results["sq_node"]
			So(ok, ShouldBeTrue)
			So(sqRes["out"], ShouldEqual, 25.0)
		})
	})
}
