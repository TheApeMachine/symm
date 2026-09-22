package compiler

import (
	"context"
	"encoding/json"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store/tables"
	"github.com/theapemachine/symm/nomagique/temporal"
)

type failingAddition struct{ arithmetic.AddServer }

func (server *failingAddition) Done(ctx context.Context, call arithmetic.Add_done) error {
	return errnie.Error(errnie.Err(errnie.IO, "fixture: terminal result failed", nil))
}

func TestProgramExecute(t *testing.T) {
	Convey("Given a terminal capability whose done call fails", t, func() {
		registry := NewRegistry()
		registry.Register("test.FailingAddition", Factory{
			InterfaceID: arithmetic.Add_TypeID,
			New: func(ctx context.Context, config []byte) (capnp.Client, error) {
				return capnp.Client(arithmetic.Add_ServerToClient(&failingAddition{})), nil
			},
		})
		program, err := CompileJSON([]byte(`{"nodes":{"terminal":{"id":"terminal","type":"test.FailingAddition"}}}`), registry, nil)
		So(err, ShouldBeNil)
		defer program.Release()

		Convey("Then execution preserves the terminal failure and node identity", func() {
			err := program.Execute(context.Background(), nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "terminal result failed")
			So(err.Error(), ShouldContainSubstring, `node "terminal"`)
		})
	})
}

func TestProgramCarriedPayload(t *testing.T) {
	Convey("Given a continuously circulating loop and an external source", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{
   "loop":{"id":"loop","type":"controlflow.Loop","inputData":{"data":{"value":"ping"},"active":{"value":true}}},
   "socket":{"id":"socket","type":"websocket.WebSocketClient"}
  }}`), nil, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		So(program.Execute(context.Background(), nil), ShouldBeNil)

		Convey("Then circulating bytes do not count as incoming activity", func() {
			So(program.carriedPayload(), ShouldBeFalse)
		})

		Convey("When the external source has received bytes", func() {
			source := program.Nodes[program.NodeMap["socket"]]
			So(source.Source, ShouldBeTrue)
			_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)
			result, err := capnp.NewRootStruct(segment, source.Done.ResultSize)
			So(err, ShouldBeNil)
			field := source.Outputs["frame.read"]
			result.SetUint16(capnp.DataOffset(field.DiscriminantOffset*2), field.DiscriminantValue)
			So(result.SetData(uint16(source.Outputs["frame.read"].Offset), []byte("received frame")), ShouldBeNil)
			program.results[source.ID] = result
			So(program.carriedPayload(), ShouldBeTrue)

			Convey("Then consuming the bytes restores idle detection", func() {
				So(program.Execute(context.Background(), nil), ShouldBeNil)
				So(program.carriedPayload(), ShouldBeFalse)
			})
		})
	})
}

func BenchmarkProgramExecute(b *testing.B) {
	program, err := CompileJSON([]byte(`{"nodes":{"add":{"id":"add","type":"arithmetic.Add","inputData":{"a":{"value":2},"b":{"value":3}}}}}`), nil, nil)

	if err != nil {
		b.Fatal(err)
	}
	defer program.Release()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := program.Execute(context.Background(), nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProgramCarriedPayload(b *testing.B) {
	// Exercise a circulating control value beside an idle source, without network I/O.
	program, err := CompileJSON([]byte(`{"nodes":{
  "loop":{"id":"loop","type":"controlflow.Loop","inputData":{"data":{"value":"ping"},"active":{"value":true}}},
  "socket":{"id":"socket","type":"websocket.WebSocketClient"}
 }}`), nil, nil)

	if err != nil {
		b.Fatal(err)
	}
	defer program.Release()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := program.Execute(context.Background(), nil); err != nil {
			b.Fatal(err)
		}

		if program.carriedPayload() {
			b.Fatal("idle graph reported external activity")
		}
	}
}

func TestProgramExecuteResource(t *testing.T) {
	Convey("Given a capability-only Transform bound into a consumer", t, func() {
		registry := NewRegistry()
		registry.Register("test.Transform", Factory{InterfaceID: data.Transform_TypeID, New: func(ctx context.Context, config []byte) (capnp.Client, error) {
			return capnp.Client(data.Transform_ServerToClient(data.NewScale(ctx))), nil
		}})
		registry.Register("data.Map", DefaultRegistry().ResolveMust("data.Map"))
		program, err := CompileJSON([]byte(`{"nodes":{
  "transform":{"id":"transform","type":"test.Transform","connections":{"outputs":{"self":[{"nodeId":"map","portName":"body"}]}}},
  "map":{"id":"map","type":"data.Map","inputData":{"data":{"value":"{\"values\":[1,2,3]}"},"path":{"value":"values"}},"connections":{"inputs":{"body":[{"nodeId":"transform","portName":"self"}]}}}
  }}`), registry, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		resource := program.Nodes[program.NodeMap["transform"]]
		So(resource.Resource, ShouldBeTrue)
		So(program.Roots, ShouldNotContain, resource.Index)
		for range 3 {
			So(program.Execute(context.Background(), nil), ShouldBeNil)
			_, found := program.Result("transform")
			So(found, ShouldBeFalse)
			value, found := program.Result("map")
			So(found, ShouldBeTrue)
			result := data.Map_done_Results(value)
			So(result.Count(), ShouldEqual, 3)
		}
	})
}

func TestProgramExecuteExcursion(t *testing.T) {
	Convey("Given a consumer of completed excursion values", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{
   "excursion":{"id":"excursion","type":"temporal.Excursion","connections":{"outputs":{"move.anchor":[{"nodeId":"consumer","portName":"a"}],"move.ignition":[{"nodeId":"consumer","portName":"b"}]}}},
   "consumer":{"id":"consumer","type":"arithmetic.Add","connections":{"inputs":{"a":[{"nodeId":"excursion","portName":"move.anchor"}],"b":[{"nodeId":"excursion","portName":"move.ignition"}]}}}
  }}`), nil, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		delivered := 0
		price := 100.0

		for step := range 100 {
			rate := 0.005

			if step >= 60 {
				rate = -0.005
			}

			price *= 1 + rate
			_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)
			params, err := temporal.NewExcursion_write_Params(segment)
			So(err, ShouldBeNil)
			params.SetValue(price)
			So(program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["excursion"]: capnp.Struct(params)}), ShouldBeNil)
			source, found := program.Result("excursion")
			So(found, ShouldBeTrue)
			_, ran := program.Result("consumer")
			move := temporal.ExcursionResult(source).Which() == temporal.ExcursionResult_Which_move
			So(ran, ShouldEqual, move)
			_, err = program.Float64Result("excursion", "move.anchor")

			if !move {
				So(err, ShouldNotBeNil)
			}

			if move {
				So(err, ShouldBeNil)
			}

			if ran {
				delivered++
			}
		}

		So(delivered, ShouldBeGreaterThan, 0)
		So(delivered, ShouldBeLessThan, 100)
	})
}

type failingDurable struct {
	tables.IcebergTableServer
	calls int
}

func (owner *failingDurable) Flush(ctx context.Context, call runtime.Durable_flush) error {
	owner.calls++
	return errnie.Error(errnie.Err(errnie.IO, "fixture: durability unavailable", nil))
}

func TestProgramFlush(t *testing.T) {
	Convey("Given a durable owner whose flush fails", t, func() {
		owner := &failingDurable{}
		registry := NewRegistry()
		registry.Register("test.Durable", Factory{InterfaceID: tables.IcebergTable_TypeID,
			New: func(ctx context.Context, config []byte) (capnp.Client, error) {
				return capnp.Client(tables.IcebergTable_ServerToClient(owner)), nil
			},
		})
		program, err := CompileJSON([]byte(`{"nodes":{"archive":{"id":"archive","type":"test.Durable"}}}`), registry, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		err = program.Flush(context.Background())
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "archive")
		So(err.Error(), ShouldContainSubstring, "durability unavailable")
		So(owner.calls, ShouldEqual, 1)
	})
}

func TestProgramExecuteIdleCapture(t *testing.T) {
	Convey("Given a source wired to capture through the frame arm", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{
 "feed":{"id":"feed","type":"websocket.WebSocketClient","connections":{"outputs":{
  "frame.read":[{"nodeId":"capture","portName":"payload"}],
  "frame.endpoint":[{"nodeId":"capture","portName":"endpoint"}],
  "frame.receivedAt":[{"nodeId":"capture","portName":"receivedAt"}]
 }}},
 "capture":{"id":"capture","type":"store.Capture","connections":{"inputs":{
  "payload":[{"nodeId":"feed","portName":"frame.read"}],
  "endpoint":[{"nodeId":"feed","portName":"frame.endpoint"}],
  "receivedAt":[{"nodeId":"feed","portName":"frame.receivedAt"}]
 }}}
}}`), nil)
		So(err, ShouldBeNil)
		defer program.Release()

		Convey("Then repeated idle polls do not execute capture with empty data", func() {
			for observation := 0; observation < 3; observation++ {
				So(program.Execute(context.Background(), nil), ShouldBeNil)
				_, captured := program.results["capture"]
				So(captured, ShouldBeFalse)
				So(program.carriedPayload(), ShouldBeFalse)
			}
		})
	})
}

/* The archive and live programs use the existing signal definitions. */
func TestProgramExecuteSignals(t *testing.T) {
	Convey("Given the training signal JSON and actual channel-tagged records", t, func() {
		ctx := context.Background()
		replay := func(repository DefinitionRepository) []float64 {
			program, err := Compile(trainingSignals(t, repository), nil, repository)
			So(err, ShouldBeNil)
			defer program.Release()
			records := program.NodeMap["records"]
			spreads := make([]float64, 0, 3)
			for _, payload := range []string{
				`{"channel":"ticker","data":[{"symbol":"BTC/USD","last":101,"bid":100,"ask":102},{"symbol":"ETH/USD","last":201,"bid":200,"ask":203}]}`,
				`{"channel":"ticker","data":[{"symbol":"BTC/USD","last":106,"bid":104,"ask":108}]}`,
				"",
			} {
				_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
				So(err, ShouldBeNil)
				args, err := data.NewIterate_write_Params(segment)
				So(err, ShouldBeNil)
				So(args.SetPath("data"), ShouldBeNil)
				args.SetEnvelope(true)
				if payload != "" {
					input, err := args.NewData(1)
					So(err, ShouldBeNil)
					So(input.Set(0, []byte(payload)), ShouldBeNil)
				}
				So(program.Execute(ctx, map[NodeID]capnp.Struct{records: capnp.Struct(args)}), ShouldBeNil)
				value, err := program.Float64Result("definition-liquidity_ticker__spread", "out")
				So(err, ShouldBeNil)
				spreads = append(spreads, value)
			}
			return spreads
		}
		Convey("Then every record reaches the existing liquidity graph", func() {
			So(replay(DefaultRepository()), ShouldResemble, []float64{2, 3, 4})
		})
		Convey("When one training connection is changed from ask to last in JSON", func() {
			repository := NewRepository()
			graph, err := repository.Load("training")
			So(err, ShouldBeNil)
			signal := graph.Nodes["definition-liquidity_ticker"]
			signal.Connections.Inputs["spread.a"] = []ConnectionTarget{{NodeID: "grid", PortName: "values_1"}}
			grid := graph.Nodes["grid"]
			for port, targets := range grid.Connections.Outputs {
				retained := make([]ConnectionTarget, 0, len(targets))
				for _, target := range targets {
					if target.NodeID == signal.ID && target.PortName == "spread.a" {
						continue
					}
					retained = append(retained, target)
				}
				grid.Connections.Outputs[port] = retained
			}
			grid.Connections.Outputs["values_1"] = append(grid.Connections.Outputs["values_1"], ConnectionTarget{NodeID: signal.ID, PortName: "spread.a"})
			encoded, err := json.Marshal(graph)
			So(err, ShouldBeNil)
			So(repository.Save("training", encoded), ShouldBeNil)
			So(replay(repository), ShouldResemble, []float64{1, 1, 2})
		})
	})
}

func BenchmarkProgramExecuteSignals(b *testing.B) {
	program, err := Compile(trainingSignals(b, DefaultRepository()), nil, DefaultRepository())
	if err != nil {
		b.Fatal(err)
	}
	defer program.Release()
	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		b.Fatal(err)
	}
	args, err := data.NewIterate_write_Params(segment)
	if err != nil {
		b.Fatal(err)
	}
	args.SetEnvelope(true)
	if err := args.SetPath("data"); err != nil {
		b.Fatal(err)
	}
	payload, err := args.NewData(1)
	if err != nil {
		b.Fatal(err)
	}
	if err := payload.Set(0, []byte(`{"channel":"ticker","data":[{"symbol":"BTC/USD","last":100,"bid":99,"ask":101,"bid_qty":2,"ask_qty":3}]}`)); err != nil {
		b.Fatal(err)
	}
	inputs := map[NodeID]capnp.Struct{program.NodeMap["records"]: capnp.Struct(args)}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := program.Execute(context.Background(), inputs); err != nil {
			b.Fatal(err)
		}
	}
}

/* trainingSignals keeps the real training computation and injects its I/O boundary. */
func trainingSignals(t testing.TB, repository DefinitionRepository) Graph {
	t.Helper()
	graph, err := repository.Load("training")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"replay", "tape", "mine", "events"} {
		delete(graph.Nodes, id)
	}
	records := graph.Nodes["records"]
	records.Connections.Inputs = nil
	graph.Nodes["records"] = records
	return graph
}
