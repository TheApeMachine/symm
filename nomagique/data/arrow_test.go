package data_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/* arrowInput builds the actual typed IPC input shared by projection proofs. */
func arrowInput(t testing.TB) []byte {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{{Name: "sequence", Type: arrow.PrimitiveTypes.Int64}, {Name: "payload", Type: arrow.BinaryTypes.Binary}}, nil)
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer builder.Release()
	var encoded bytes.Buffer
	writer := ipc.NewWriter(&encoded, ipc.WithSchema(schema))
	for index := int64(0); index < 2; index++ {
		builder.Field(0).(*array.Int64Builder).Append(9007199254740993 + index)
		builder.Field(1).(*array.BinaryBuilder).Append([]byte{byte(index), 0, 255})
		batch := builder.NewRecordBatch()
		err := writer.Write(batch)
		batch.Release()
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

/* TestArrowWrite verifies indexing across batches, exact values and absent input. */
func TestArrowWrite(t *testing.T) {
	Convey("Given a two-batch IPC stream", t, func() {
		ctx := context.Background()
		payload := arrowInput(t)
		client := data.Arrow_ServerToClient(data.NewArrow())
		defer client.Release()
		for index := uint64(0); index < 2; index++ {
			So(client.Write(ctx, func(args data.Arrow_write_Params) error {
				args.SetRow(index)
				return args.SetData(payload)
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			raw, err := result.Out()
			So(err, ShouldBeNil)
			var row struct {
				Sequence int64
				Payload  []byte
			}
			So(json.Unmarshal(raw, &row), ShouldBeNil)
			So(row.Sequence, ShouldEqual, 9007199254740993+int64(index))
			So(row.Payload, ShouldResemble, []byte{byte(index), 0, 255})
			release()
		}
		Convey("A row outside the stream is an error", func() {
			So(client.Write(ctx, func(args data.Arrow_write_Params) error { args.SetRow(2); return args.SetData(payload) }), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
		Convey("An absent frame does not repeat the previous projection", func() {
			So(client.Write(ctx, nil), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			raw, err := result.Out()
			So(err, ShouldBeNil)
			So(raw, ShouldBeEmpty)
		})
	})
}

/* BenchmarkArrowWrite measures IPC decoding and exact row projection through the capability. */
func BenchmarkArrowWrite(b *testing.B) {
	payload := arrowInput(b)
	ctx := context.Background()
	client := data.Arrow_ServerToClient(data.NewArrow())
	defer client.Release()
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Write(ctx, func(args data.Arrow_write_Params) error { args.SetRow(1); return args.SetData(payload) }); err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}
