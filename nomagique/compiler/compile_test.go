package compiler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	gorillaws "github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/network/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/store/tables"
	crypto "github.com/theapemachine/symm/nomagique/transport/crypto"
)

func TestCompileFlume(t *testing.T) {
	Convey("Given the typed Cap'n Proto Flume compiler", t, func() {
		reg := compiler.DefaultRegistry()
		So(reg, ShouldNotBeNil)

		Convey("Test B: Multi-input graph invocation with same evaluation", func() {
			graph := compiler.Graph{
				ID:   "multi_input_test",
				Name: "multi_input_test",
				Nodes: map[string]compiler.Node{
					"left": {
						ID:   "left",
						Type: "test.Float64Source",
						Connections: compiler.Connections{
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "add", PortName: "a"}},
							},
						},
					},
					"right": {
						ID:   "right",
						Type: "test.Float64Source",
						Connections: compiler.Connections{
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "add", PortName: "b"}},
							},
						},
					},
					"add": {
						ID:   "add",
						Type: "arithmetic.Add",
						Connections: compiler.Connections{
							Inputs: map[string][]compiler.ConnectionTarget{
								"a": {{NodeID: "left", PortName: "out"}},
								"b": {{NodeID: "right", PortName: "out"}},
							},
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "collect", PortName: "value"}},
							},
						},
					},
					"collect": {
						ID:   "collect",
						Type: "statistic.Mean",
						Connections: compiler.Connections{
							Inputs: map[string][]compiler.ConnectionTarget{
								"value": {{NodeID: "add", PortName: "out"}},
							},
						},
					},
				},
			}

			program, err := compiler.Compile(graph, reg)
			So(err, ShouldBeNil)
			So(program, ShouldNotBeNil)

			leftIdx := program.NodeMap["left"]
			rightIdx := program.NodeMap["right"]

			_, segL, _ := capnp.NewMessage(capnp.SingleSegment(nil))
			inL, _ := capnp.NewRootStruct(segL, capnp.ObjectSize{DataSize: 8})
			inL.SetUint64(0, math.Float64bits(2.0))

			_, segR, _ := capnp.NewMessage(capnp.SingleSegment(nil))
			inR, _ := capnp.NewRootStruct(segR, capnp.ObjectSize{DataSize: 8})
			inR.SetUint64(0, math.Float64bits(2.0))

			err = program.Execute(context.Background(), map[compiler.NodeID]capnp.Struct{
				leftIdx:  inL,
				rightIdx: inR,
			})
			So(err, ShouldBeNil)

			res, err := program.Float64Result("add", "out")
			So(err, ShouldBeNil)
			So(res, ShouldEqual, 4.0)

			Convey("Test C: a fan-in node waits for every wire", func() {
				addIdx, ok := program.NodeMap["add"]
				So(ok, ShouldBeTrue)

				add := program.Nodes[addIdx]

				left := add.Inputs["a"].Index
				right := add.Inputs["b"].Index

				// Both wires are required, so neither arriving alone makes the
				// node runnable. Readiness is the mask, not the argument
				// struct, which is what keeps a missing input from being read
				// as a zero one.
				So(add.RequiredMask&(1<<left), ShouldNotEqual, 0)
				So(add.RequiredMask&(1<<right), ShouldNotEqual, 0)
				So(add.RequiredMask&(1<<left), ShouldNotEqual, add.RequiredMask)
			})
		})

		Convey("Test E: Type mismatch between incompatible ports fails at compile time", func() {
			customReg := compiler.NewRegistry()
			customReg.Register("crypto.Base64Encode", compiler.Factory{
				InterfaceID: crypto.Base64Encode_TypeID,
				New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
					return capnp.Client(crypto.Base64Encode_ServerToClient(crypto.NewBase64Encode(ctx))), nil
				},
			})
			customReg.Register("arithmetic.Add", compiler.Factory{
				InterfaceID: compiler.DefaultRegistry().ResolveMust("arithmetic.Add").InterfaceID,
				New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
					return compiler.DefaultRegistry().ResolveMust("arithmetic.Add").New(ctx, cfg)
				},
			})

			mismatchGraph := compiler.Graph{
				ID:   "mismatch_test",
				Name: "mismatch_test",
				Nodes: map[string]compiler.Node{
					"textSrc": {
						ID:   "textSrc",
						Type: "crypto.Base64Encode",
						Connections: compiler.Connections{
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "add", PortName: "a"}},
							},
						},
					},
					"add": {
						ID:   "add",
						Type: "arithmetic.Add",
						Connections: compiler.Connections{
							Inputs: map[string][]compiler.ConnectionTarget{
								"a": {{NodeID: "textSrc", PortName: "out"}},
							},
						},
					},
				},
			}

			_, err := compiler.Compile(mismatchGraph, customReg)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "type mismatch")
		})

		Convey("Test F: End-to-end JSON graph (source -> atanh -> add -> sink)", func() {
			endToEndGraph := compiler.Graph{
				ID:   "e2e_atanh_add",
				Name: "e2e_atanh_add",
				Nodes: map[string]compiler.Node{
					"src": {
						ID:   "src",
						Type: "test.Float64Source",
						Connections: compiler.Connections{
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "atanh", PortName: "value"}},
							},
						},
					},
					"atanh": {
						ID:   "atanh",
						Type: "calculus.Atanh",
						Connections: compiler.Connections{
							Inputs: map[string][]compiler.ConnectionTarget{
								"value": {{NodeID: "src", PortName: "out"}},
							},
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "add", PortName: "a"}},
							},
						},
					},
					"add": {
						ID:   "add",
						Type: "arithmetic.Add",
						InputData: map[string]json.RawMessage{
							"b": json.RawMessage(`{"float": 1.0}`),
						},
						Connections: compiler.Connections{
							Inputs: map[string][]compiler.ConnectionTarget{
								"a": {{NodeID: "atanh", PortName: "out"}},
							},
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "collect", PortName: "value"}},
							},
						},
					},
					"collect": {
						ID:   "collect",
						Type: "statistic.Mean",
						Connections: compiler.Connections{
							Inputs: map[string][]compiler.ConnectionTarget{
								"value": {{NodeID: "add", PortName: "out"}},
							},
						},
					},
				},
			}

			prog, err := compiler.Compile(endToEndGraph, reg)
			So(err, ShouldBeNil)
			So(prog, ShouldNotBeNil)

			srcIdx := prog.NodeMap["src"]
			inputVal := 0.5

			_, segS, _ := capnp.NewMessage(capnp.SingleSegment(nil))
			inS, _ := capnp.NewRootStruct(segS, capnp.ObjectSize{DataSize: 8})
			inS.SetUint64(0, math.Float64bits(inputVal))

			err = prog.Execute(context.Background(), map[compiler.NodeID]capnp.Struct{
				srcIdx: inS,
			})
			So(err, ShouldBeNil)

			finalResult, err := prog.Float64Result("add", "out")
			So(err, ShouldBeNil)
			expected := math.Atanh(inputVal) + 1.0
			So(finalResult, ShouldAlmostEqual, expected, 1e-6)
		})

		Convey("Test G: Strict type copier rejects 64-bit cross-type coercion (Float64 to Int64)", func() {
			fromField := compiler.FieldInfo{Name: "src", Offset: 0, Which: schema.Type_Which_float64}
			toField := compiler.FieldInfo{Name: "dst", Offset: 0, Which: schema.Type_Which_int64}
			copier, err := compiler.CompileCopier(fromField, toField)
			So(err, ShouldNotBeNil)
			So(copier, ShouldBeNil)
			So(err.Error(), ShouldContainSubstring, "type mismatch")
		})

		Convey("Test H: Union-member routing activates only matching discriminant", func() {
			p := &compiler.Program{
				Nodes: []compiler.CompiledNode{
					{
						ID:     "src",
						Index:  0,
						Origin: true,
					},
					{
						ID:           "target1",
						Index:        1,
						RequiredMask: 1 << 0,
					},
					{
						ID:           "target2",
						Index:        2,
						RequiredMask: 1 << 0,
					},
				},
				Routes: []compiler.Route{
					{
						FromNode:       0,
						FromField:      0,
						ToNode:         1,
						ToField:        0,
						FromInUnion:    true,
						FromDiscVal:    0,
						FromDiscOffset: 0,
					},
					{
						FromNode:       0,
						FromField:      1,
						ToNode:         2,
						ToField:        0,
						FromInUnion:    true,
						FromDiscVal:    1,
						FromDiscOffset: 0,
					},
				},
				NodeMap: map[string]compiler.NodeID{
					"src":     0,
					"target1": 1,
					"target2": 2,
				},
			}

			// In source struct, set discriminant to 0 (targeting target1 only)
			_, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
			srcStruct, _ := capnp.NewRootStruct(seg, capnp.ObjectSize{DataSize: 8})
			srcStruct.SetUint16(0, 0) // discriminant 0 at offset 0

			err := p.Execute(context.Background(), map[compiler.NodeID]capnp.Struct{
				0: srcStruct,
			})
			So(err, ShouldBeNil)

			// target1 received the route and ran
			_, target1Ran := p.Result("target1")
			So(target1Ran, ShouldBeTrue)

			// target2 was on the inactive union branch (discVal 1 != 0), so it never ran
			_, target2Ran := p.Result("target2")
			So(target2Ran, ShouldBeFalse)
		})

		Convey("Test I: Compiled Flume graph with real Cap'n Proto union primitive only activates selected route", func() {
			ctx := context.Background()

			runScenario := func(chooseYes bool) (targetYesRan bool, targetNoRan bool) {
				reg := compiler.DefaultRegistry()
				branchServer := compiler.NewTestUnionServer(chooseYes)
				reg.Register("test.Branch", compiler.Factory{
					InterfaceID: compiler.TestUnion_TypeID,
					New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
						return capnp.Client(compiler.TestUnion_ServerToClient(branchServer)), nil
					},
				})
				voidSink := compiler.NewTestSinkVoidServer()
				reg.Register("test.SinkVoid", compiler.Factory{
					InterfaceID: compiler.TestSinkVoid_TypeID,
					New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
						return capnp.Client(compiler.TestSinkVoid_ServerToClient(voidSink)), nil
					},
				})

				flumeJSON := `{
					"nodes": {
						"src": {
							"id": "src",
							"type": "test.Float64Source",
							"connections": {
								"outputs": {
									"out": [{"nodeId": "branch", "portName": "in"}]
								}
							}
						},
						"branch": {
							"id": "branch",
							"type": "test.Branch",
							"connections": {
								"inputs": {
									"in": [{"nodeId": "src", "portName": "out"}]
								},
								"outputs": {
									"yes": [{"nodeId": "target_yes", "portName": "a"}],
									"no": [{"nodeId": "target_no", "portName": "in"}]
								}
							}
						},
						"target_yes": {
							"id": "target_yes",
							"type": "arithmetic.Add",
							"inputData": {
								"b": {"float": 1.0}
							},
							"connections": {
								"inputs": {
									"a": [{"nodeId": "branch", "portName": "yes"}]
								}
							}
						},
						"target_no": {
							"id": "target_no",
							"type": "test.SinkVoid",
							"connections": {
								"inputs": {
									"in": [{"nodeId": "branch", "portName": "no"}]
								}
							}
						}
					}
				}`

				var graph compiler.Graph
				unmarshalErr := json.Unmarshal([]byte(flumeJSON), &graph)
				So(unmarshalErr, ShouldBeNil)

				prog, compErr := compiler.CompileWithPrevious(graph, reg, nil, nil)
				So(compErr, ShouldBeNil)
				So(prog, ShouldNotBeNil)

				_, seg, msgErr := capnp.NewMessage(capnp.SingleSegment(nil))
				So(msgErr, ShouldBeNil)
				srcStruct, stErr := capnp.NewRootStruct(seg, capnp.ObjectSize{DataSize: 8})
				So(stErr, ShouldBeNil)
				srcStruct.SetUint64(0, math.Float64bits(42.0))

				execErr := prog.Execute(ctx, map[compiler.NodeID]capnp.Struct{
					prog.NodeMap["src"]: srcStruct,
				})
				So(execErr, ShouldBeNil)

				_, yesOk := prog.Result("target_yes")
				_, noOk := prog.Result("target_no")
				return yesOk, noOk
			}

			// When branch selects 'yes': only target_yes executes
			yesRan, noRan := runScenario(true)
			So(yesRan, ShouldBeTrue)
			So(noRan, ShouldBeFalse)

			// When branch selects 'no': only target_no executes
			yesRan2, noRan2 := runScenario(false)
			So(yesRan2, ShouldBeFalse)
			So(noRan2, ShouldBeTrue)
		})

		Convey("Test J: Unified multi-domain compilation partitions UI and backend lowering", func() {
			mixedJSON := `{
				"id": "mixed_graph",
				"name": "mixed_graph",
				"nodes": {
					"calc": {
						"id": "calc",
						"type": "arithmetic.Add",
						"connections": {
							"outputs": {
								"out": [
									{"nodeId": "meter", "portName": "value"}
								]
							}
						}
					},
					"panel": {
						"id": "panel",
						"type": "ui.Panel",
						"inputData": {
							"variant": {"value": "sunken"},
							"className": {"value": "p-4"}
						},
						"connections": {
							"inputs": {
								"components": [
									{"nodeId": "meter", "portName": "out"}
								]
							}
						}
					},
					"meter": {
						"id": "meter",
						"type": "ui.Meter",
						"connections": {
							"inputs": {
								"value": [
									{"nodeId": "calc", "portName": "out"}
								]
							},
							"outputs": {
								"out": [
									{"nodeId": "panel", "portName": "components"}
								]
							}
						}
					}
				}
			}`

			var graph compiler.Graph
			unmarshalErr := json.Unmarshal([]byte(mixedJSON), &graph)
			So(unmarshalErr, ShouldBeNil)

			prog, err := compiler.Compile(graph, reg)
			So(err, ShouldBeNil)
			So(prog, ShouldNotBeNil)

			// Backend execution contains only backend nodes
			So(len(prog.Nodes), ShouldEqual, 1)
			So(prog.Nodes[0].ID, ShouldEqual, "calc")

			// UI plan contains the structural hierarchy
			So(prog.UI, ShouldNotBeNil)
			So(len(prog.UI.Routes), ShouldEqual, 1)
			So(len(prog.UI.Routes[0].Components), ShouldEqual, 1)
			rootNode := prog.UI.Routes[0].Components[0]
			So(rootNode.Name, ShouldEqual, "Panel")
			So(rootNode.ClassName, ShouldEqual, "p-4")
			So(len(rootNode.Children), ShouldEqual, 1)
			So(rootNode.Children[0].Name, ShouldEqual, "Meter")

			// Binding plan captures cross-domain edge: calc.out -> meter.value
			So(prog.Bindings, ShouldNotBeNil)
			So(len(prog.Bindings.Bindings), ShouldEqual, 1)
			So(prog.Bindings.Bindings[0].SourceNode, ShouldEqual, "calc")
			So(prog.Bindings.Bindings[0].SourcePort, ShouldEqual, "out")
			So(prog.Bindings.Bindings[0].TargetNode, ShouldEqual, "meter")
			So(prog.Bindings.Bindings[0].TargetProp, ShouldEqual, "value")
		})
	})
}

func TestCompileJSON(t *testing.T) {
	Convey("Given authored static values", t, func() {
		Convey("An invalid numeric value reports the node and port", func() {
			program, err := compiler.CompileJSON([]byte(`{"nodes":{"sum":{"id":"sum","type":"arithmetic.Add","inputData":{"a":{"value":"not a number"}}}}}`), nil, nil)
			So(program, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, `invalid static input "a" on node "sum"`)
		})
		Convey("A null numeric control is missing rather than zero", func() {
			program, err := compiler.CompileJSON([]byte(`{"nodes":{"sum":{"id":"sum","type":"arithmetic.Add","inputData":{"a":{"value":null}}}}}`), nil, nil)
			So(program, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "static input has no value")
		})
		Convey("A missing schema field cannot silently disappear", func() {
			program, err := compiler.CompileJSON([]byte(`{"nodes":{"sum":{"id":"sum","type":"arithmetic.Add","inputData":{"missing":{"value":12}}}}}`), nil, nil)
			So(program, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, `no input port "missing"`)
		})
		Convey("A valid zero remains an authored value", func() {
			program, err := compiler.CompileJSON([]byte(`{"nodes":{"sum":{"id":"sum","type":"arithmetic.Add","inputData":{"a":{"value":0},"b":{"value":7}}}}}`), nil, nil)
			So(err, ShouldBeNil)
			defer program.Release()
			So(program.Execute(context.Background(), nil), ShouldBeNil)
			value, err := program.Float64Result("sum", "out")
			So(err, ShouldBeNil)
			So(value, ShouldEqual, 7)
		})
	})
}

func BenchmarkCompileJSON(b *testing.B) {
	payload := []byte(`{"nodes":{"sum":{"id":"sum","type":"arithmetic.Add","inputData":{"a":{"value":0},"b":{"value":7}}}}}`)
	b.ReportAllocs()

	for b.Loop() {
		program, err := compiler.CompileJSON(payload, nil, nil)

		if err != nil {
			b.Fatal(err)
		}
		program.Release()
	}
}

/* TestCompileTraining verifies capture and metric replay have separate input boundaries. */
func TestCompileTraining(t *testing.T) {
	Convey("Given separate capture and metric replay programs", t, func() {
		for _, name := range []string{"capture", "training"} {
			program, err := compiler.CompileFile("../../manifest/"+name+".json", nil, compiler.DefaultRepository())
			So(err, ShouldBeNil)
			defer program.Release()

			if name == "capture" {
				// All three sockets (spot, level3, futures) record into one
				// session and one table, and nothing that grades or learns
				// runs inside capture.
				kinds := map[string]int{}

				for _, node := range program.Nodes {
					kinds[node.Identity.Type]++
					So(node.Identity.Type, ShouldNotStartWith, "paper.")
					So(node.Identity.Type, ShouldNotStartWith, "cognition.")
				}
				So(kinds["store.Capture"], ShouldEqual, 1)
				So(kinds["tables.IcebergTable"], ShouldEqual, 1)
				So(kinds["websocket.WebSocketClient"], ShouldEqual, 5)
				continue
			}

			_, restored := program.NodeMap["gather"]
			So(restored, ShouldBeTrue)
			_, queried := program.NodeMap["query"]
			So(queried, ShouldBeTrue)
			_, raw := program.NodeMap["signals__grid"]
			So(raw, ShouldBeFalse)

			for _, node := range program.Nodes {
				So(node.Identity.Type, ShouldNotEqual, "cognition.Reinforce")
				So(node.Identity.Type, ShouldNotEqual, "learning.TaskLearner")
				So(node.Identity.Type, ShouldNotEqual, "websocket.WebSocketClient")
			}
		}
	})
}

func TestParseGraphAgreement(t *testing.T) {
	Convey("Given a graph whose two halves disagree about an edge", t, func() {
		Convey("A reader nobody sends to is refused", func() {
			_, err := compiler.ParseGraph([]byte(`{
				"id": "disagree", "name": "disagree",
				"nodes": {
					"a": {"id": "a", "type": "arithmetic.Add",
						"connections": {"inputs": {}, "outputs": {}}},
					"b": {"id": "b", "type": "calculus.Square",
						"connections": {
							"inputs": {"value": [{"nodeId": "a", "portName": "out"}]},
							"outputs": {}
						}}
				}
			}`))

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "does not send it")
		})

		Convey("A sender nobody reads is refused", func() {
			_, err := compiler.ParseGraph([]byte(`{
				"id": "disagree", "name": "disagree",
				"nodes": {
					"a": {"id": "a", "type": "arithmetic.Add",
						"connections": {
							"inputs": {},
							"outputs": {"out": [{"nodeId": "b", "portName": "value"}]}
						}},
					"b": {"id": "b", "type": "calculus.Square",
						"connections": {"inputs": {}, "outputs": {}}}
				}
			}`))

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "does not read it")
		})

		Convey("Both halves agreeing is accepted", func() {
			_, err := compiler.ParseGraph([]byte(`{
				"id": "agree", "name": "agree",
				"nodes": {
					"a": {"id": "a", "type": "arithmetic.Add",
						"connections": {
							"inputs": {},
							"outputs": {"out": [{"nodeId": "b", "portName": "value"}]}
						}},
					"b": {"id": "b", "type": "calculus.Square",
						"connections": {
							"inputs": {"value": [{"nodeId": "a", "portName": "out"}]},
							"outputs": {}
						}}
				}
			}`))

			So(err, ShouldBeNil)
		})

		Convey("Direct Compile rejects asymmetric graph", func() {
			disagreeGraph := compiler.Graph{
				ID:   "disagree",
				Name: "disagree",
				Nodes: map[string]compiler.Node{
					"a": {
						ID:   "a",
						Type: "arithmetic.Add",
						Connections: compiler.Connections{
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "b", PortName: "value"}},
							},
						},
					},
					"b": {
						ID:   "b",
						Type: "calculus.Square",
					},
				},
			}

			_, err := compiler.Compile(disagreeGraph, nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "does not read it")
		})
	})
}

/* stageCounter observes actual calls reached through compiled node capabilities. */
type stageCounter struct {
	count   atomic.Int64
	entered chan struct{}
	gate    <-chan struct{}
}

func (stage *stageCounter) Step(ctx context.Context, call runtime.StageNode_step) error {
	if stage.entered != nil {
		stage.entered <- struct{}{}
	}
	if stage.gate != nil {
		select {
		case <-stage.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	stage.count.Add(1)
	return nil
}

func workspaceNodeGraph() compiler.Graph {
	nodes := map[string]compiler.Node{
		"z-stage":  {ID: "z-stage", Type: "test.Stage"},
		"y-first":  {ID: "y-first", Type: "runtime.Consumer"},
		"x-second": {ID: "x-second", Type: "runtime.Consumer"},
		"w-group":  {ID: "w-group", Type: "runtime.Group"},
		"a-workspace": {ID: "a-workspace", Type: "runtime.Workspace", InputData: map[string]json.RawMessage{
			"capacity": json.RawMessage(`8`), "writers": json.RawMessage(`1`), "epoch": json.RawMessage(`77`), "admit": json.RawMessage(`true`),
		}},
	}
	connect := func(provider, consumer, port string) {
		source, destination := nodes[provider], nodes[consumer]

		if source.Connections.Outputs == nil {
			source.Connections.Outputs = map[string][]compiler.ConnectionTarget{}
		}

		if destination.Connections.Inputs == nil {
			destination.Connections.Inputs = map[string][]compiler.ConnectionTarget{}
		}
		source.Connections.Outputs["self"] = append(source.Connections.Outputs["self"], compiler.ConnectionTarget{NodeID: consumer, PortName: port})
		destination.Connections.Inputs[port] = []compiler.ConnectionTarget{{NodeID: provider, PortName: "self"}}
		nodes[provider], nodes[consumer] = source, destination
	}
	connect("z-stage", "y-first", "target")
	connect("z-stage", "x-second", "target")
	connect("y-first", "w-group", "consumers_0")
	connect("x-second", "w-group", "consumers_1")
	connect("w-group", "a-workspace", "groups_0")
	return compiler.Graph{ID: "workspace-nodes", Name: "workspace-nodes", Nodes: nodes}
}

func TestCompileCapabilities(t *testing.T) {
	Convey("Given a graph whose names sort against capability initialization order", t, func() {
		registry := compiler.DefaultRegistry()
		stage := &stageCounter{}
		registry.Register("test.Stage", compiler.Factory{InterfaceID: runtime.StageNode_TypeID, New: func(ctx context.Context, config []byte) (capnp.Client, error) {
			return stage.client(), nil
		}})
		program, err := compiler.Compile(workspaceNodeGraph(), registry, compiler.DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()

		Convey("List ports retain both consumers in numbered order", func() {
			group := program.Nodes[program.NodeMap["w-group"]]
			members, err := runtime.Group_write_Params(group.ArgsTemplate).Consumers()
			So(err, ShouldBeNil)
			So(members.Len(), ShouldEqual, 2)
			first, err := members.At(0)
			So(err, ShouldBeNil)
			second, err := members.At(1)
			So(err, ShouldBeNil)
			So(capnp.Client(first).IsSame(program.Nodes[program.NodeMap["y-first"]].Client), ShouldBeTrue)
			So(capnp.Client(second).IsSame(program.Nodes[program.NodeMap["x-second"]].Client), ShouldBeTrue)
		})

		Convey("The graph configures dependencies and LMAX calls both consumer nodes", func() {
			ctx := context.Background()
			err := program.Execute(ctx, nil)
			So(err, ShouldBeNil)
			workspace := runtime.Workspace(program.Nodes[program.NodeMap["a-workspace"]].Client)
			So(workspace.Write(ctx, func(params runtime.Workspace_write_Params) error {
				data, err := params.NewData(1)

				if err != nil {
					return err
				}
				return data.Set(0, []byte(`{"channel":"ticker"}`))
			}), ShouldBeNil)
			So(workspace.WaitStreaming(), ShouldBeNil)
			future, release := workspace.Flush(ctx, nil)
			defer release()
			_, err = future.Struct()
			So(err, ShouldBeNil)
			So(stage.count.Load(), ShouldEqual, 2)
		})
	})
}

func TestCompileCaptureWiring(t *testing.T) {
	Convey("Every socket in the production and capture graphs reaches the raw archive", t, func() {
		for _, definition := range []string{"system", "capture"} {
			graph, err := compiler.DefaultRepository().Load(definition)
			So(err, ShouldBeNil)
			program, err := compiler.Compile(graph, nil, compiler.DefaultRepository())
			So(err, ShouldBeNil)
			defer program.Release()
			captureIndex := program.NodeMap["envelope"]
			capture := store.Capture_ServerToClient(store.NewCapture(context.Background()))
			defer capture.Release()
			sources := []string{"spot__socket", "level3__shard_0__socket", "futures__socket", "level3__shard_1__socket", "level3__shard_2__socket"}

			// A second generation represents reconnect snapshots from every socket.
			for generation := range 2 {
				So(capture.Write(context.Background(), func(params store.Capture_write_Params) error {
					for slot, source := range sources {
						message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

						if err != nil {
							return err
						}
						defer message.Release()
						received, err := websocket.NewRootReceived(segment)

						if err != nil {
							return err
						}
						received.SetFrame()
						frame := received.Frame()
						frame.SetGeneration(uint64(generation + 1))
						for _, err := range []error{
							frame.SetRead([]byte(fmt.Sprintf("snapshot:%s:%d", source, generation))),
							frame.SetProvenance([]byte(fmt.Sprintf(`{"session":%q,"sequence":%d,"endpoint":%q,"receivedAt":"2026-09-26T00:00:00Z"}`, source, generation, "wss://fixture/"+source))),
							frame.SetEndpoint("wss://fixture/" + source),
							frame.SetReceivedAt(time.Date(2026, 9, 26, 0, 0, generation, slot, time.UTC).Format(time.RFC3339Nano)),
						} {
							if err != nil {
								return err
							}
						}
						copied := 0

						for _, route := range program.Routes {
							if route.FromNode != program.NodeMap[source] || route.ToNode != captureIndex {
								continue
							}

							if err := route.Copy(capnp.Struct(received), capnp.Struct(params)); err != nil {
								return err
							}
							copied++
						}

						if copied != 2 {
							return fmt.Errorf("%s: expected payload and source provenance routes, got %d", source, copied)
						}
					}
					return nil
				}), ShouldBeNil)
				So(capture.WaitStreaming(), ShouldBeNil)

				for slot, source := range sources {
					future, release := capture.Done(context.Background(), nil)
					result, err := future.Struct()
					So(err, ShouldBeNil)
					So(result.Which(), ShouldEqual, store.Captured_Which_row)
					payload, err := result.Row().Payload()
					So(err, ShouldBeNil)
					So(string(payload), ShouldEqual, fmt.Sprintf("snapshot:%s:%d", source, generation))
					endpoint, err := result.Row().Endpoint()
					So(err, ShouldBeNil)
					So(endpoint, ShouldEqual, "wss://fixture/"+source)
					So(result.Row().Sequence(), ShouldEqual, generation)
					So(result.Pending(), ShouldEqual, len(sources)-slot-1)
					release()
				}
			}
		}
	})
}

func TestCompileFuturesReconnect(t *testing.T) {
	Convey("The authored futures node sends both subscriptions on every connection", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		received := make(chan string, 4)
		failures := make(chan error, 4)
		upgrader := gorillaws.Upgrader{}
		venue := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			connection, err := upgrader.Upgrade(writer, request, nil)

			if err != nil {
				failures <- err
				return
			}
			defer func() {
				if err := connection.Close(); err != nil {
					failures <- err
				}
			}()

			for range 2 {
				_, payload, err := connection.ReadMessage()

				if err != nil {
					failures <- err
					return
				}
				select {
				case received <- string(payload):
				case <-ctx.Done():
					return
				}
			}
		}))
		defer venue.Close()
		graph, err := compiler.DefaultRepository().Load("live_futures")
		So(err, ShouldBeNil)
		endpoint, err := json.Marshal("ws" + strings.TrimPrefix(venue.URL, "http"))
		So(err, ShouldBeNil)
		socket := graph.Nodes["socket"]
		socket.InputData["endpoint"] = endpoint
		graph.Nodes["socket"] = socket
		program, err := compiler.Compile(graph, nil, compiler.DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		So(program.Execute(ctx, nil), ShouldBeNil)

		for range 2 {
			for _, feed := range []string{"ticker", "trade"} {
				select {
				case payload := <-received:
					var subscription struct {
						Event    string   `json:"event"`
						Feed     string   `json:"feed"`
						Products []string `json:"product_ids"`
					}
					So(json.Unmarshal([]byte(payload), &subscription), ShouldBeNil)
					So(subscription.Event, ShouldEqual, "subscribe")
					So(subscription.Feed, ShouldEqual, feed)
					So(subscription.Products, ShouldResemble, []string{"PI_XBTUSD"})
				case err := <-failures:
					So(err, ShouldBeNil)
				case <-ctx.Done():
					t.Fatal("futures subscription did not arrive: " + feed)
				}
			}
		}
	})
}

func TestCompileQuery(t *testing.T) {
	Convey("The shipping analytical node is configured by the graph without catalog I/O", t, func() {
		graph, err := compiler.DefaultRepository().Load("system")
		So(err, ShouldBeNil)
		node := graph.Nodes["inspection"]
		node.Connections = compiler.Connections{}
		graph.Nodes = map[string]compiler.Node{"inspection": node}
		program, err := compiler.Compile(graph, nil, compiler.DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		So(program.Execute(context.Background(), nil), ShouldBeNil)
		client := tables.Query(program.Nodes[program.NodeMap["inspection"]].Client)
		future, release := client.Done(context.Background(), nil)
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		So(result.Configured(), ShouldBeTrue)
	})
}

func (stage *stageCounter) Fence(ctx context.Context, call runtime.StageNode_fence) error { return nil }

func TestCompileConfigured(t *testing.T) {
	Convey("Configured capabilities do not turn root admission into Consumer polling", t, func() {
		gate := make(chan struct{})
		stage := &stageCounter{entered: make(chan struct{}, 4), gate: gate}
		registry := compiler.DefaultRegistry()
		registry.Register("test.Stage", compiler.Factory{InterfaceID: runtime.StageNode_TypeID, New: func(context.Context, []byte) (capnp.Client, error) {
			return stage.client(), nil
		}})
		graph := workspaceNodeGraph()
		workspaceNode := graph.Nodes["a-workspace"]
		workspaceNode.InputData["data"] = json.RawMessage(`["market"]`)
		graph.Nodes["a-workspace"] = workspaceNode
		program, err := compiler.Compile(graph, registry)
		So(err, ShouldBeNil)
		defer program.Release()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		So(program.Execute(ctx, nil), ShouldBeNil)
		select {
		case <-stage.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		completed := make(chan error, 1)
		go func() { completed <- program.Execute(ctx, nil) }()
		select {
		case err := <-completed:
			So(err, ShouldBeNil)
		case <-ctx.Done():
			close(gate)
			t.Fatal("root admission waited for a running stage")
		}
		close(gate)
		So(program.Flush(ctx), ShouldBeNil)
		So(stage.count.Load(), ShouldEqual, 4)
	})
}

func (stage *stageCounter) client() capnp.Client {
	server := runtime.StageNode_NewServer(stage)
	server.NewArena = func() capnp.Arena { return capnp.MultiSegment(nil) }
	return capnp.NewClient(server)
}
