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
	"github.com/theapemachine/symm/nomagique/transport"
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
								"out": {{NodeID: "sink", PortName: "value"}},
							},
						},
					},
					"sink": {
						ID:   "sink",
						Type: "data.Sink",
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

			Convey("Test C: Incomplete invocation (send only add.a)", func() {
				_, segSingle, _ := capnp.NewMessage(capnp.SingleSegment(nil))
				inSingle, _ := capnp.NewRootStruct(segSingle, capnp.ObjectSize{DataSize: 8})
				inSingle.SetUint64(0, math.Float64bits(2.0))

				err := program.Execute(context.Background(), map[compiler.NodeID]capnp.Struct{
					leftIdx: inSingle,
				})
				So(err, ShouldBeNil)

				// Add.write was NOT invoked because add.b was never supplied
				_, addRan := program.Result("add")
				So(addRan, ShouldBeFalse)
			})
		})

		Convey("Test E: Type mismatch between incompatible ports fails at compile time", func() {
			customReg := compiler.NewRegistry()
			customReg.Register("transport.Base64Encode", compiler.Factory{
				InterfaceID: transport.Base64Encode_TypeID,
				New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
					return capnp.Client(transport.Base64Encode_ServerToClient(transport.NewBase64Encode())), nil
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
						Type: "transport.Base64Encode",
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
								"out": {{NodeID: "atanh", PortName: "in"}},
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
								"out": {{NodeID: "sink", PortName: "value"}},
							},
						},
					},
					"sink": {
						ID:   "sink",
						Type: "data.Sink",
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
						ID:       "src",
						Index:    0,
						IsSource: true,
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
	})
}
