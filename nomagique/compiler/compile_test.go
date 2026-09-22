package compiler_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
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
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "collect", PortName: "value"}},
							},
						},
					},
					"collect": {
						ID:   "collect",
						Type: "statistic.Mean",
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
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "collect", PortName: "value"}},
							},
						},
					},
					"collect": {
						ID:   "collect",
						Type: "statistic.Mean",
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
						ID:    "src",
						Index: 0,
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
