package websocket

import (
	"context"
	"fmt"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* shardFixture owns deterministic child capabilities; integration tests use real sockets. */
type shardFixture struct{ children []*shardStage }
type shardStage struct {
	members []string
	idle    bool
}

func (fixture *shardFixture) Create(ctx context.Context, call runtime.StageFactory_create) error {
	child := &shardStage{}
	fixture.children = append(fixture.children, child)
	client := runtime.StageNode_ServerToClient(child)
	defer client.Release()
	result, err := call.AllocResults()
	if err != nil {
		return err
	}
	return result.SetStage(client.AddRef())
}
func (child *shardStage) Step(ctx context.Context, call runtime.StageNode_step) error {
	values, err := call.Args().Texts()
	if err != nil {
		return err
	}
	for index := range values.Len() {
		value, err := values.At(index)
		if err != nil {
			return err
		}
		child.members = append(child.members, value)
	}
	result, err := call.AllocResults()
	if err != nil {
		return err
	}
	outputs, err := result.NewOutputs(1)
	if err != nil {
		return err
	}
	output := outputs.At(0)
	output.SetInterfaceId(WebSocketClient_TypeID)
	if err := output.SetNode("socket"); err != nil {
		return err
	}
	frame, err := NewReceived(result.Segment())
	if err != nil {
		return err
	}
	if child.idle {
		frame.SetIdle()
		return output.SetValue(capnp.Struct(frame).ToPtr())
	}
	frame.SetFrame()
	frame.Frame().SetGeneration(7)
	if err := frame.Frame().SetRead([]byte(`{"channel":"level3","data":[]}`)); err != nil {
		return err
	}
	if err := frame.Frame().SetProvenance([]byte("original-source-stamp")); err != nil {
		return err
	}
	return output.SetValue(capnp.Struct(frame).ToPtr())
}
func (child *shardStage) Fence(context.Context, runtime.StageNode_fence) error { return nil }

/* TestShardsWrite crosses multiple former limits and preserves existing membership on updates. */
func TestShardsWrite(t *testing.T) {
	Convey("Symbols create as many independent graph nodes as their venue capacity requires", t, func() {
		fixture := &shardFixture{}
		factory := runtime.StageFactory_ServerToClient(fixture)
		defer factory.Release()
		server := NewShards()
		client := Shards_ServerToClient(server)
		defer client.Release()
		submit := func(count int) {
			So(client.Write(context.Background(), func(args Shards_write_Params) error {
				args.SetCapacity(200)
				if err := args.SetFactory(factory.AddRef()); err != nil {
					return err
				}
				values, err := args.NewSymbols(int32(count))
				if err != nil {
					return err
				}
				for index := range count {
					if err := values.Set(index, fmt.Sprintf("COIN%d/USD", index)); err != nil {
						return err
					}
				}
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
		}
		poll := func(count int) {
			for range count {
				future, release := client.Done(context.Background(), nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)
				So(result.Which(), ShouldEqual, Received_Which_frame)
				So(result.Frame().Generation(), ShouldEqual, 7)
				provenance, err := result.Frame().Provenance()
				So(err, ShouldBeNil)
				So(string(provenance), ShouldEqual, "original-source-stamp")
				release()
			}
		}
		submit(1001)
		poll(6)
		So(len(fixture.children), ShouldEqual, 6)
		submit(1001)
		poll(6)
		submit(1203)
		poll(7)
		So(len(fixture.children), ShouldEqual, 7)
		seen := map[string]bool{}
		for index, child := range fixture.children {
			expected := 200
			if index == 6 {
				expected = 3
			}
			So(len(child.members), ShouldEqual, expected)
			for _, symbol := range child.members {
				So(seen[symbol], ShouldBeFalse)
				seen[symbol] = true
			}
		}
		So(len(seen), ShouldEqual, 1203)
	})
}

/* BenchmarkShardsDone exercises fair native result transfer across six active capabilities. */
func BenchmarkShardsDone(b *testing.B) {
	server := NewShards()
	for range 6 {
		server.shards = append(server.shards, shard{stage: runtime.StageNode_ServerToClient(&shardStage{})})
	}
	client := Shards_ServerToClient(server)
	defer client.Release()
	b.ReportAllocs()
	for b.Loop() {
		future, release := client.Done(context.Background(), nil)
		if _, err := future.Struct(); err != nil {
			release()
			b.Fatal(err)
		}
		release()
	}
}

/* TestShardsDone prevents a quiet shard from hiding runnable work in another shard. */
func TestShardsDone(t *testing.T) {
	Convey("Idle is reported only after every shard was polled", t, func() {
		server := NewShards()
		for _, idle := range []bool{true, false} {
			server.shards = append(server.shards, shard{stage: runtime.StageNode_ServerToClient(&shardStage{idle: idle})})
		}
		client := Shards_ServerToClient(server)
		defer client.Release()
		for range 2 {
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Which(), ShouldEqual, Received_Which_frame)
			release()
		}
	})
}
