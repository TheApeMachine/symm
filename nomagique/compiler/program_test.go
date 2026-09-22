package compiler

import (
	"context"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/arithmetic"
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
