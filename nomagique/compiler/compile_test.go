package compiler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	"github.com/apache/iceberg-go"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/store/tables"
	"github.com/theapemachine/symm/nomagique/temporal"
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

/* TestCompileTraining verifies the offline repair boundary, not a surrogate learner. */
func TestCompileTraining(t *testing.T) {
	Convey("Given the separated capture and offline mining programs", t, func() {
		for _, name := range []string{"capture", "training"} {
			program, err := compiler.CompileFile("../../manifest/"+name+".json", nil, compiler.DefaultRepository())
			So(err, ShouldBeNil)
			defer program.Release()

			if name == "capture" {
				So(program.Nodes, ShouldHaveLength, 3)
				So(program.Routes, ShouldHaveLength, 4)
				So(program.Nodes[program.Roots[0]].ID, ShouldEqual, "feed")
				continue
			}

			_, signalPresent := program.NodeMap["definition-sentiment_ticker__return"]
			So(signalPresent, ShouldBeTrue)
			_, replacementPresent := program.NodeMap["measure"]
			So(replacementPresent, ShouldBeFalse)

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
	})
}

func TestCompile(t *testing.T) {
	Convey("Given the training manifest and a real multi-file Iceberg archive", t, func() {
		ctx := context.Background()
		directory := t.TempDir()
		database, err := sql.Open("sqlite3", filepath.Join(directory, "catalog.db"))
		So(err, ShouldBeNil)
		t.Cleanup(func() {
			if err := database.Close(); err != nil {
				t.Error(err)
			}
		})
		underlying, err := sqlcat.NewCatalog("test", database, sqlcat.SQLite, iceberg.Properties{"warehouse": "file://" + directory})
		So(err, ShouldBeNil)
		So(underlying.CreateNamespace(ctx, []string{"symm"}, nil), ShouldBeNil)
		catalog := underlying
		source, err := os.ReadFile("../../manifest/training.json")
		So(err, ShouldBeNil)
		graph, err := compiler.ParseGraph(source)
		So(err, ShouldBeNil)
		declaration := func(node string) string {
			var value struct {
				Value string `json:"value"`
			}
			So(json.Unmarshal(graph.Nodes[node].InputData["config"], &value), ShouldBeNil)
			return value.Value
		}
		captureGraph, err := compiler.DefaultRepository().Load("capture")
		So(err, ShouldBeNil)
		var captureConfig struct {
			Value string `json:"value"`
		}
		So(json.Unmarshal(captureGraph.Nodes["capture"].InputData["config"], &captureConfig), ShouldBeNil)
		inputConfig := captureConfig.Value
		outputConfig := declaration("events")
		capture := store.Capture_ServerToClient(store.NewCapture())
		defer capture.Release()
		rows := make([][]byte, 0, 180)

		// The first record is flat; only the second symbol traverses three sustained legs.
		for index := 0; index < 180; index++ {
			offset := index % 60

			exponent := float64(offset) * 0.005

			if index/60 == 1 {
				exponent = 59*0.005 - float64(offset)*0.008
			}

			if index/60 == 2 {
				exponent = 59*0.005 - 59*0.008 + float64(offset)*0.008
			}
			payload, err := json.Marshal(map[string]any{"channel": "ticker", "data": []map[string]any{{"symbol": "FLAT/USD", "last": 200, "bid": 199, "ask": 201}, {"symbol": "MOVE/USD", "last": 100 * math.Exp(exponent), "bid": 100 * math.Exp(exponent), "ask": 100*math.Exp(exponent) + 3}}})
			So(err, ShouldBeNil)
			So(capture.Write(ctx, func(params store.Capture_write_Params) error {
				for _, err := range []error{params.SetEndpoint("wss://fixture"), params.SetReceivedAt("2026-09-22T12:00:00.123456789Z"), params.SetPayload(payload)} {
					if err != nil {
						return err
					}
				}
				return nil
			}), ShouldBeNil)
			So(capture.WaitStreaming(), ShouldBeNil)
			future, release := capture.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			row, err := result.Out()
			So(err, ShouldBeNil)
			rows = append(rows, bytes.Clone(row))
			release()
		}
		archive := tables.NewIcebergTable()
		archive.Catalog = catalog
		writer := tables.IcebergTable_ServerToClient(archive)
		defer writer.Release()
		// Reverse storage order and repeat an identity across separate committed files.
		rows = append(rows, rows[10])

		for index := len(rows) - 1; index >= 0; index-- {
			So(writer.Write(ctx, func(params tables.IcebergTable_write_Params) error {
				params.SetCommit(index%30 == 0)

				if err := params.SetConfig(inputConfig); err != nil {
					return err
				}
				return params.SetPayload(rows[index])
			}), ShouldBeNil)
			So(writer.WaitStreaming(), ShouldBeNil)
		}
		future, release := runtime.Durable(writer).Flush(ctx, nil)
		_, err = future.Struct()
		release()
		So(err, ShouldBeNil)

		registry := compiler.DefaultRegistry()
		archiveTable, err := catalog.LoadTable(ctx, []string{"symm", "raw_frames_v3"})
		So(err, ShouldBeNil)
		var catalogRequests, tableRequests atomic.Int64
		endpoint := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			response.Header().Set("Content-Type", "application/json")
			if request.URL.Path == "/config" {
				catalogRequests.Add(1)
				if err := json.NewEncoder(response).Encode(map[string]any{"defaults": map[string]string{}, "overrides": map[string]string{}}); err != nil {
					t.Error(err)
				}
				return
			}
			tableRequests.Add(1)
			if err := json.NewEncoder(response).Encode(map[string]any{"metadata": archiveTable.Metadata(), "config": map[string]string{}}); err != nil {
				t.Error(err)
			}
		}))
		defer endpoint.Close()
		input, err := json.Marshal(map[string]any{"catalogUrl": endpoint.URL + "/config", "tableUrl": endpoint.URL + "/table", "properties": map[string]string{}})
		So(err, ShouldBeNil)
		configured, err := json.Marshal(map[string]string{"value": string(input)})
		So(err, ShouldBeNil)
		replay := graph.Nodes["replay"]
		replay.InputData = map[string]json.RawMessage{"input.through": configured}
		graph.Nodes["replay"] = replay

		registry.Register("tables.IcebergTable", compiler.Factory{InterfaceID: tables.IcebergTable_TypeID, New: func(ctx context.Context, config []byte) (capnp.Client, error) {
			writer := tables.NewIcebergTable()
			writer.Catalog = catalog
			return capnp.Client(tables.IcebergTable_ServerToClient(writer)), nil
		}})
		program, err := compiler.Compile(graph, registry, compiler.DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()

		spreads := make([]float64, 0, 360)
		graded := make([]temporal.TapeCursor, 0)
		for observation := 0; observation < len(rows)+360+360*3+1; observation++ {
			So(program.Execute(ctx, nil), ShouldBeNil)
			if result, found := program.Result("grade__labels"); found && data.Iterate_done_Results(result).Found() {
				payload, err := data.Iterate_done_Results(result).Out()
				So(err, ShouldBeNil)
				var label struct {
					Cursor temporal.TapeCursor
					Event  temporal.MinedEvent
					Market struct{ Data struct{ Symbol string } }
					Truth  struct {
						Action  string
						Holding bool
					}
				}
				So(json.Unmarshal(payload, &label), ShouldBeNil)
				So(label.Market.Data.Symbol, ShouldEqual, "MOVE/USD")
				So(label.Event.A, ShouldNotBeNil)
				So(label.Cursor.Compare(*label.Event.A), ShouldBeGreaterThanOrEqualTo, 0)
				So(label.Cursor.Compare(label.Event.C), ShouldBeLessThan, 0)
				So(label.Truth.Action, ShouldBeIn, "WAIT", "EXIT")
				So(label.Truth.Holding, ShouldEqual, label.Cursor.Compare(label.Event.B) <= 0)
				graded = append(graded, label.Cursor)
			}
			const spreadNode = "definition-liquidity_ticker__spread"
			if _, produced := program.Result(spreadNode); produced {
				spread, err := program.Float64Result(spreadNode, "out")
				So(err, ShouldBeNil)
				spreads = append(spreads, spread)
			}
		}
		So(catalogRequests.Load(), ShouldEqual, 1)
		So(tableRequests.Load(), ShouldEqual, 1)
		So(spreads, ShouldHaveLength, 360)
		for index, spread := range spreads {
			So(spread, ShouldAlmostEqual, 2+index%2)
		}

		So(program.Flush(ctx), ShouldBeNil)

		scanner := tables.NewIcebergScan()
		var eventDeclaration struct{ Namespace, Table string }
		So(json.Unmarshal([]byte(outputConfig), &eventDeclaration), ShouldBeNil)
		eventTable, err := catalog.LoadTable(ctx, []string{eventDeclaration.Namespace, eventDeclaration.Table})
		So(err, ShouldBeNil)
		eventMetadata, err := json.Marshal(eventTable.Metadata())
		So(err, ShouldBeNil)
		reader := tables.IcebergScan_ServerToClient(scanner)
		defer reader.Release()
		So(reader.Write(ctx, func(params tables.IcebergScan_write_Params) error {
			metadata, err := params.NewMetadata(1)
			if err != nil {
				return err
			}
			if err := metadata.Set(0, eventMetadata); err != nil {
				return err
			}
			properties, err := params.NewProperties(1)
			if err != nil {
				return err
			}
			return properties.Set(0, []byte(`{}`))
		}), ShouldBeNil)
		So(reader.WaitStreaming(), ShouldBeNil)
		events := make([]temporal.MinedEvent, 0)

		for {
			future, release := reader.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)

			if result.Exhausted() {
				release()
				break
			}
			raw, err := result.Out()
			So(err, ShouldBeNil)
			projector := data.Arrow_ServerToClient(data.NewArrow())
			So(projector.Write(ctx, func(args data.Arrow_write_Params) error { return args.SetData(raw) }), ShouldBeNil)
			So(projector.WaitStreaming(), ShouldBeNil)
			projected, releaseProjection := projector.Done(ctx, nil)
			projection, err := projected.Struct()
			So(err, ShouldBeNil)
			row, err := projection.Out()
			So(err, ShouldBeNil)
			defer releaseProjection()
			defer projector.Release()
			var archived struct {
				Payload []byte `json:"payload"`
			}
			So(json.Unmarshal(row, &archived), ShouldBeNil)
			var batch []temporal.MinedEvent
			So(json.Unmarshal(archived.Payload, &batch), ShouldBeNil)
			events = append(events, batch...)
			release()
		}
		So(events, ShouldHaveLength, 2)

		for _, event := range events {
			So(event.Symbol, ShouldEqual, "MOVE/USD")
			So(event.B.Record, ShouldEqual, 1)
			So(event.C.Sequence, ShouldBeGreaterThan, event.B.Sequence)
			So(event.D.Sequence, ShouldBeGreaterThan, event.C.Sequence)
		}
		So(events[0].Excursion, ShouldBeGreaterThan, 0)
		So(events[0].A, ShouldBeNil)
		So(events[1].Excursion, ShouldBeLessThan, 0)
		So(events[1].A, ShouldNotBeNil)
		So(events[1].A.Sequence, ShouldBeLessThan, events[1].B.Sequence)
		So(len(graded), ShouldEqual, events[1].C.Sequence-events[1].A.Sequence)
	})
}
