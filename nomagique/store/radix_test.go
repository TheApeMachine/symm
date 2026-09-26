package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestRadixWrite(t *testing.T) {
	Convey("Given one radix owner with atomic keyed batches", t, func() {
		client := store.Radix_ServerToClient(store.NewRadix())
		defer client.Release()

		for _, operation := range []struct {
			keys, values, previous []string
			found                  []bool
		}{
			{[]string{"model", "example"}, nil, []string{"", ""}, []bool{false, false}},
			{[]string{"model", "example"}, []string{"counts", "truth"}, []string{"", ""}, []bool{false, false}},
			{[]string{"example", "model"}, nil, []string{"truth", "counts"}, []bool{true, true}},
			{[]string{"model"}, []string{"{}"}, []string{"counts"}, []bool{true}},
			{[]string{"model", "missing"}, nil, []string{"{}", ""}, []bool{true, false}},
		} {
			So(client.Write(context.Background(), func(params store.Radix_write_Params) error {
				keys, err := params.NewKey(int32(len(operation.keys)))

				if err != nil {
					return err
				}

				for index, key := range operation.keys {
					if err := keys.Set(index, key); err != nil {
						return err
					}
				}

				if operation.values == nil {
					return nil
				}

				values, err := params.NewValue(int32(len(operation.values)))

				if err != nil {
					return err
				}

				for index, value := range operation.values {
					if err := values.Set(index, []byte(value)); err != nil {
						return err
					}
				}

				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			values, err := result.Out()
			So(err, ShouldBeNil)
			found, err := result.Found()
			So(err, ShouldBeNil)

			for index, expected := range operation.previous {
				value, err := values.At(index)
				So(err, ShouldBeNil)
				So(string(value), ShouldEqual, expected)
				So(found.At(index), ShouldEqual, operation.found[index])
			}

			release()
		}
	})

	Convey("Given a batch whose second replacement is missing", t, func() {
		server := store.NewRadix()
		writer := store.Radix_ServerToClient(server)
		err := writer.Write(context.Background(), func(params store.Radix_write_Params) error {
			keys, err := params.NewKey(2)

			if err != nil {
				return err
			}

			if err := keys.Set(0, "model"); err != nil {
				return err
			}

			if err := keys.Set(1, "example"); err != nil {
				return err
			}

			values, err := params.NewValue(2)

			if err != nil {
				return err
			}

			return values.Set(0, []byte("must not commit"))
		})

		if err == nil {
			err = writer.WaitStreaming()
		}

		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "every replacement slot")
		writer.Release()

		// A streaming failure closes that RPC stream; a new connection to the
		// same owner must still see neither half of the rejected transaction.
		reader := store.Radix_ServerToClient(server)
		defer reader.Release()
		So(reader.Write(context.Background(), func(params store.Radix_write_Params) error {
			keys, err := params.NewKey(2)

			if err != nil {
				return err
			}

			if err := keys.Set(0, "model"); err != nil {
				return err
			}

			return keys.Set(1, "example")
		}), ShouldBeNil)
		So(reader.WaitStreaming(), ShouldBeNil)
		future, release := reader.Done(context.Background(), nil)
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		found, err := result.Found()
		So(err, ShouldBeNil)
		So(found.At(0), ShouldBeFalse)
		So(found.At(1), ShouldBeFalse)
	})
}

/* TestRadixFlush verifies durable replacement through the node's public capability. */
func TestRadixFlush(t *testing.T) {
	Convey("A checkpoint retains successive model revisions across node lifetimes", t, func() {
		path := filepath.Join(t.TempDir(), "model.capnp")
		for _, revision := range []string{"first", "second"} {
			client := store.Radix_ServerToClient(store.NewRadix())
			So(client.Write(context.Background(), func(params store.Radix_write_Params) error {
				if err := params.SetPath(path); err != nil {
					return err
				}
				keys, err := params.NewKey(1)
				if err != nil {
					return err
				}
				if err := keys.Set(0, "model"); err != nil {
					return err
				}
				values, err := params.NewValue(1)
				if err != nil {
					return err
				}
				return values.Set(0, []byte(revision))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Flush(context.Background(), nil)
			_, err := future.Struct()
			So(err, ShouldBeNil)
			release()
			client.Release()
			restored := store.Radix_ServerToClient(store.NewRadix())
			So(restored.Write(context.Background(), func(params store.Radix_write_Params) error {
				if err := params.SetPath(path); err != nil {
					return err
				}
				keys, err := params.NewKey(1)
				if err != nil {
					return err
				}
				return keys.Set(0, "model")
			}), ShouldBeNil)
			So(restored.WaitStreaming(), ShouldBeNil)
			selected, release := restored.Done(context.Background(), nil)
			result, err := selected.Struct()
			So(err, ShouldBeNil)
			values, err := result.Out()
			So(err, ShouldBeNil)
			value, err := values.At(0)
			So(err, ShouldBeNil)
			So(string(value), ShouldEqual, revision)
			release()
			restored.Release()
		}
		Convey("A different authored key contract cannot silently reuse an older model", func() {
			client := store.Radix_ServerToClient(store.NewRadix())
			defer client.Release()
			So(client.Write(context.Background(), func(params store.Radix_write_Params) error {
				if err := params.SetContract("different-key-schema"); err != nil {
					return err
				}
				return params.SetPath(path)
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
		Convey("A corrupted checkpoint fails instead of silently starting an empty model", func() {
			So(os.WriteFile(path, []byte("corrupt"), 0600), ShouldBeNil)
			client := store.Radix_ServerToClient(store.NewRadix())
			defer client.Release()
			So(client.Write(context.Background(), func(params store.Radix_write_Params) error { return params.SetPath(path) }), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
		Convey("An unwritable destination preserves the last complete checkpoint", func() {
			client := store.Radix_ServerToClient(store.NewRadix())
			defer client.Release()
			So(client.Write(context.Background(), func(params store.Radix_write_Params) error {
				if err := params.SetPath(path); err != nil {
					return err
				}
				keys, err := params.NewKey(1)
				if err != nil {
					return err
				}
				if err := keys.Set(0, "model"); err != nil {
					return err
				}
				values, err := params.NewValue(1)
				if err != nil {
					return err
				}
				return values.Set(0, []byte("third"))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			backup := path + ".previous"
			So(os.Rename(path, backup), ShouldBeNil)
			So(os.Mkdir(path, 0700), ShouldBeNil)
			future, release := client.Flush(context.Background(), nil)
			_, err := future.Struct()
			So(err, ShouldNotBeNil)
			release()
			So(os.Remove(path), ShouldBeNil)
			So(os.Rename(backup, path), ShouldBeNil)
			future, release = client.Flush(context.Background(), nil)
			_, err = future.Struct()
			So(err, ShouldBeNil)
			release()
		})
	})
}

/* BenchmarkRadixFlush measures actual model updates and durable checkpoints. */
func BenchmarkRadixFlush(b *testing.B) {
	client := store.Radix_ServerToClient(store.NewRadix())
	defer client.Release()
	path := filepath.Join(b.TempDir(), "model.capnp")
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Write(ctx, func(params store.Radix_write_Params) error {
			if err := params.SetPath(path); err != nil {
				return err
			}
			keys, err := params.NewKey(1)
			if err != nil {
				return err
			}
			if err := keys.Set(0, "BTC/USD/vocabulary/A/B/flat"); err != nil {
				return err
			}
			values, err := params.NewValue(1)
			if err != nil {
				return err
			}
			return values.Set(0, []byte(`{"ENTER":42,"WAIT":13}`))
		}); err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Flush(ctx, nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}
