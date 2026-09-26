package compiler

import (
	"bytes"
	"context"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
)

/* factorySnapshotFixture seeds the actual retained vector inside two independent graphs. */
func factorySnapshotFixture(t testing.TB, populate bool) runtime.StageFactory {
	t.Helper()
	graph := []byte(`{"id":"state","nodes":{"state":{"id":"state","type":"store.Vector"}}}`)
	factory := &stageFactory{graph: graph, children: map[string]runtime.State{}}
	if populate {
		for index, name := range []string{"BTC/USD", "ETH/USD"} {
			program, err := CompileJSON(graph, nil)
			if err != nil {
				t.Fatal(err)
			}
			vector := store.Vector(program.Nodes[program.NodeMap["state"]].Client)
			if err := vector.Write(context.Background(), func(args store.Vector_write_Params) error {
				args.SetWidth(1)
				positions, err := args.NewIndex(1)
				if err != nil {
					return err
				}
				positions.Set(0, 0)
				values, err := args.NewValues(1)
				if err != nil {
					return err
				}
				values.Set(0, float64(index+1))
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := vector.WaitStreaming(); err != nil {
				t.Fatal(err)
			}
			factory.children[name] = runtime.State_ServerToClient(program)
		}
	}
	return runtime.StageFactory_ServerToClient(factory)
}

func TestStageFactorySnapshot(t *testing.T) {
	Convey("A partition checkpoint retains each graph's native state", t, func() {
		ctx := context.Background()
		factory := factorySnapshotFixture(t, true)
		defer factory.Release()
		future, release := runtime.Snapshot(factory).Snapshot(ctx, nil)
		result, err := future.Struct()
		So(err, ShouldBeNil)
		encoded, err := result.Data()
		So(err, ShouldBeNil)
		saved := bytes.Clone(encoded)
		release()
		message, err := capnp.Unmarshal(saved)
		So(err, ShouldBeNil)
		defer message.Release()
		state, err := runtime.ReadRootSnapshotSet(message)
		So(err, ShouldBeNil)
		entries, err := state.Entries()
		So(err, ShouldBeNil)
		So(entries.Len(), ShouldEqual, 2)
		for index := range entries.Len() {
			graph, err := entries.At(index).Data()
			So(err, ShouldBeNil)
			child, err := capnp.Unmarshal(graph)
			So(err, ShouldBeNil)
			snapshot, err := runtime.ReadRootSnapshotSet(child)
			So(err, ShouldBeNil)
			owners, err := snapshot.Entries()
			So(err, ShouldBeNil)
			So(owners.Len(), ShouldEqual, 1)
			payload, err := owners.At(0).Data()
			So(err, ShouldBeNil)
			retained, err := capnp.Unmarshal(payload)
			So(err, ShouldBeNil)
			vector, err := store.ReadRootVectorSnapshot(retained)
			So(err, ShouldBeNil)
			values, err := vector.Values()
			So(err, ShouldBeNil)
			So(values.At(0), ShouldEqual, index+1)
			retained.Release()
			child.Release()
		}
		restored := factorySnapshotFixture(t, false)
		defer restored.Release()
		loaded, release := runtime.Snapshot(restored).Restore(ctx, func(args runtime.Snapshot_restore_Params) error { return args.SetData(saved) })
		_, err = loaded.Struct()
		release()
		So(err, ShouldBeNil)
		again, release := runtime.Snapshot(restored).Snapshot(ctx, nil)
		defer release()
		restoredResult, err := again.Struct()
		So(err, ShouldBeNil)
		restoredBytes, err := restoredResult.Data()
		So(err, ShouldBeNil)
		So(restoredBytes, ShouldResemble, saved)
	})
}

func TestStageFactoryRestore(t *testing.T) {
	Convey("An invalid partition cannot replace any currently retained market", t, func() {
		ctx := context.Background()
		factory := factorySnapshotFixture(t, true)
		defer factory.Release()
		future, release := runtime.Snapshot(factory).Snapshot(ctx, nil)
		result, err := future.Struct()
		So(err, ShouldBeNil)
		encoded, err := result.Data()
		So(err, ShouldBeNil)
		saved := bytes.Clone(encoded)
		release()
		candidate, err := capnp.Unmarshal(bytes.Clone(saved))
		So(err, ShouldBeNil)
		defer candidate.Release()
		snapshot, err := runtime.ReadRootSnapshotSet(candidate)
		So(err, ShouldBeNil)
		entries, err := snapshot.Entries()
		So(err, ShouldBeNil)
		// The first candidate is valid. Restoring the second fails after allocation.
		payload, err := entries.At(1).Data()
		So(err, ShouldBeNil)
		payload[0] = 0xff // Corrupt the nested segment table while preserving the outer message.
		invalid, err := candidate.Marshal()
		So(err, ShouldBeNil)
		attempted, release := runtime.Snapshot(factory).Restore(ctx, func(args runtime.Snapshot_restore_Params) error { return args.SetData(invalid) })
		_, err = attempted.Struct()
		release()
		So(err, ShouldNotBeNil)
		after, release := runtime.Snapshot(factory).Snapshot(ctx, nil)
		defer release()
		retained, err := after.Struct()
		So(err, ShouldBeNil)
		current, err := retained.Data()
		So(err, ShouldBeNil)
		So(current, ShouldResemble, saved)
	})
}

func BenchmarkStageFactorySnapshot(b *testing.B) {
	factory := factorySnapshotFixture(b, true)
	defer factory.Release()
	b.ReportAllocs()
	for b.Loop() {
		future, release := runtime.Snapshot(factory).Snapshot(context.Background(), nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}
