package compiler

import (
	"context"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/cognition"
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
			So(result.SetData(uint16(source.Outputs["read"].Offset), []byte("received frame")), ShouldBeNil)
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
	Convey("Given capability-only memory bound into a consumer", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{
   "memory":{"id":"memory","type":"cognition.Memory","connections":{"outputs":{"self":[{"nodeId":"reinforce","portName":"memory"}]}}},
   "reinforce":{"id":"reinforce","type":"cognition.Reinforce","inputData":{"contextBytes":{"value":"precursor"},"classBytes":{"value":"ENTER"}},"connections":{"inputs":{"memory":[{"nodeId":"memory","portName":"self"}]}}}
  }}`), nil, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		resource := program.Nodes[program.NodeMap["memory"]]
		So(resource.Resource, ShouldBeTrue)
		So(program.Roots, ShouldNotContain, resource.Index)

		Convey("Repeated execution uses memory only through its consumer", func() {
			for range 3 {
				So(program.Execute(context.Background(), nil), ShouldBeNil)
				_, found := program.Result("memory")
				So(found, ShouldBeFalse)
			}
			memory := cognition.Memory(resource.Client)
			future, release := memory.Steps(context.Background(), nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Out(), ShouldEqual, 3)
		})
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
