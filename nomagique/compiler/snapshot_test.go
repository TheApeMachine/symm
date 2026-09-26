package compiler

import (
	"context"
	"path/filepath"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
)

/* seedMapState writes actual retained owners with their declared record widths. */
func seedMapState(t testing.TB, program *Program, coordinates int) {
	t.Helper()
	for name, width := range map[string]int{"previous": 1, "authority_state": 3, "pair_state": 5, "positions": 2, "partition": 4} {
		records := coordinates
		if name == "pair_state" {
			records = coordinates * (coordinates - 1) / 2
		}
		client := store.Vector(program.Nodes[program.NodeMap[name]].Client)
		if err := client.Write(context.Background(), func(params store.Vector_write_Params) error {
			params.SetWidth(uint32(width))
			indices, err := params.NewIndex(int32(records))
			if err != nil {
				return err
			}
			values, err := params.NewValues(int32(records * width))
			if err != nil {
				return err
			}
			for index := range records {
				indices.Set(index, int64(index))
			}
			for index := range records * width {
				values.Set(index, float64(index+1))
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			t.Fatal(err)
		}
	}
}

/* TestProgramSnapshot preserves the complete grid behind one durable checkpoint. */
func TestProgramSnapshot(t *testing.T) {
	Convey("A consumer fences all five grid state owners into one durable revision", t, func() {
		ctx := context.Background()
		path := filepath.Join(t.TempDir(), "grid.capnp")
		for restart := range 2 {
			program, err := CompileFile("../../manifest/impulse_map.json", nil, DefaultRepository())
			So(err, ShouldBeNil)
			server := runtime.State_NewServer(program)
			server.NewArena = func() capnp.Arena { return capnp.MultiSegment(nil) }
			target := runtime.State(capnp.NewClient(server))
			checkpoint := store.Radix_ServerToClient(store.NewRadix())
			So(checkpoint.Write(ctx, func(params store.Radix_write_Params) error { return params.SetPath(path) }), ShouldBeNil)
			So(checkpoint.WaitStreaming(), ShouldBeNil)
			consumer := runtime.Consumer_ServerToClient(runtime.NewConsumer(ctx))
			So(consumer.Write(ctx, func(params runtime.Consumer_write_Params) error {
				params.SetCheckpointReady(true)
				if err := params.SetName("grid"); err != nil {
					return err
				}
				if err := params.SetTarget(runtime.StageNode(target.AddRef())); err != nil {
					return err
				}
				if err := params.SetSnapshot(runtime.Snapshot(target.AddRef())); err != nil {
					return err
				}
				return params.SetCheckpoint(runtime.Checkpoint(checkpoint.AddRef()))
			}), ShouldBeNil)
			So(consumer.WaitStreaming(), ShouldBeNil)
			if restart == 0 {
				seedMapState(t, program, 3)
			}
			for name, width := range map[string]int{"previous": 1, "authority_state": 3, "pair_state": 5, "positions": 2, "partition": 4} {
				node := store.Vector(program.Nodes[program.NodeMap[name]].Client)
				future, release := node.Done(ctx, nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)
				values, err := result.Values()
				So(err, ShouldBeNil)
				found, err := result.Found()
				So(err, ShouldBeNil)
				So(found.Len(), ShouldEqual, 3)
				So(values.Len(), ShouldEqual, 3*width)
				for index := range values.Len() {
					So(values.At(index), ShouldEqual, float64(index+1))
				}
				release()
			}
			future, release := consumer.Fence(ctx, nil)
			_, err = future.Struct()
			So(err, ShouldBeNil)
			release()
			consumer.Release()
			target.Release()
			checkpoint.Release()
		}
	})
}

/* BenchmarkProgramSnapshot includes all 411 coordinates and their pair statistics. */
func BenchmarkProgramSnapshot(b *testing.B) {
	program, err := CompileFile("../../manifest/impulse_map.json", nil, DefaultRepository())
	if err != nil {
		b.Fatal(err)
	}
	seedMapState(b, program, 411)
	client := runtime.State_ServerToClient(program)
	defer client.Release()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		future, release := client.Snapshot(context.Background(), nil)
		result, err := future.Struct()
		if err != nil {
			release()
			b.Fatal(err)
		}
		encoded, err := result.Data()
		if err != nil {
			release()
			b.Fatal(err)
		}
		b.SetBytes(int64(len(encoded)))
		release()
	}
}
