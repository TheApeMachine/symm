package compiler_test

import (
	"context"
	"math"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/types"
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
						Type: "data.Source",
						Connections: compiler.Connections{
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "add", PortName: "a"}},
							},
						},
					},
					"right": {
						ID:   "right",
						Type: "data.Source",
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

			pipeline, err := compiler.Compile(graph, reg)
			So(err, ShouldBeNil)
			So(pipeline, ShouldNotBeNil)

			var receivedResult float64
			sinkCapability := types.NewFloat64Sink(
				func(ctx context.Context, val float64) error {
					receivedResult = val
					return nil
				},
				nil,
			)
			err = pipeline.ConnectOutput("sink", "out", sinkCapability)
			So(err, ShouldBeNil)

			ctx, _ := types.NextEvaluationContext(context.Background())
			leftSink, err := pipeline.InputSink("left", "in")
			So(err, ShouldBeNil)
			rightSink, err := pipeline.InputSink("right", "in")
			So(err, ShouldBeNil)

			err = leftSink.Write(ctx, func(p types.Float64Sink_write_Params) error {
				p.SetValue(2.0)
				return nil
			})
			So(err, ShouldBeNil)

			err = rightSink.Write(ctx, func(p types.Float64Sink_write_Params) error {
				p.SetValue(2.0)
				return nil
			})
			So(err, ShouldBeNil)

			err = pipeline.WaitStreaming()
			So(err, ShouldBeNil)
			So(receivedResult, ShouldEqual, 4.0)

			Convey("Test C: Incomplete invocation (send only add.a)", func() {
				receivedResult = -999.0
				incompleteCtx, _ := types.NextEvaluationContext(context.Background())

				err := leftSink.Write(incompleteCtx, func(p types.Float64Sink_write_Params) error {
					p.SetValue(2.0)
					return nil
				})
				So(err, ShouldBeNil)

				err = pipeline.WaitStreaming()
				So(err, ShouldBeNil)
				// Add.write was NOT invoked because add.b was never supplied for this evaluation
				So(receivedResult, ShouldEqual, -999.0)
			})

			Convey("Test D: Evaluation isolation (eval 1 receives add.a, eval 2 receives add.b)", func() {
				receivedResult = -999.0
				ctx1 := types.WithEvaluationID(context.Background(), 100)
				ctx2 := types.WithEvaluationID(context.Background(), 200)

				// Evaluation 1 provides add.a = 2.0
				err := leftSink.Write(ctx1, func(p types.Float64Sink_write_Params) error {
					p.SetValue(2.0)
					return nil
				})
				So(err, ShouldBeNil)

				// Evaluation 2 provides add.b = 3.0
				err = rightSink.Write(ctx2, func(p types.Float64Sink_write_Params) error {
					p.SetValue(3.0)
					return nil
				})
				So(err, ShouldBeNil)

				err = pipeline.WaitStreaming()
				So(err, ShouldBeNil)
				// Never combined: Add.write was not invoked for either incomplete evaluation
				So(receivedResult, ShouldEqual, -999.0)
			})
		})

		Convey("Test E: Type mismatch between incompatible ports fails at compile time", func() {
			customReg := compiler.NewRegistry()
			customReg.Register(compiler.PrimitiveDescriptor{
				Op: "text.Producer",
				OutputPorts: map[string]compiler.PortType{
					"out": compiler.PortTypeText,
				},
				Construct: func(node compiler.Node) (any, capnp.Client, error) {
					return nil, capnp.Client{}, nil
				},
			})

			mismatchGraph := compiler.Graph{
				ID:   "mismatch_test",
				Name: "mismatch_test",
				Nodes: map[string]compiler.Node{
					"textSrc": {
						ID:   "textSrc",
						Type: "text.Producer",
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
						Type: "data.Source",
						Connections: compiler.Connections{
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "atanh", PortName: "a"}},
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
						InputData: map[string]any{
							"b": 1.0,
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

			pipeline, err := compiler.Compile(endToEndGraph, reg)
			So(err, ShouldBeNil)
			So(pipeline, ShouldNotBeNil)

			var finalResult float64
			sinkCapability := types.NewFloat64Sink(
				func(ctx context.Context, val float64) error {
					finalResult = val
					return nil
				},
				nil,
			)
			err = pipeline.ConnectOutput("sink", "out", sinkCapability)
			So(err, ShouldBeNil)

			inputVal := 0.5
			ctx, _ := types.NextEvaluationContext(context.Background())

			err = pipeline.WriteFloat64(ctx, inputVal)
			So(err, ShouldBeNil)

			expected := math.Atanh(inputVal) + 1.0
			So(finalResult, ShouldEqual, expected)
		})
	})
}
