package tables

import (
	"capnproto.org/go/capnp/v3/schemas"
	"context"
	"fmt"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/types"
	"math"
)

/* nativeCutFixture uses Gather's production record and the persisted column types. */
func nativeCutFixture(t testing.TB) (types.Record, *arrow.Schema, func()) {
	t.Helper()
	data.RegisterSchema(schemas.DefaultRegistry)
	client := data.Gather_ServerToClient(data.NewGather())
	// 411 is the production vocabulary width; alternating flags exercise warming cuts.
	err := client.Write(context.Background(), func(params data.Gather_write_Params) error {
		params.SetEpoch(9007199254740993)
		params.SetSequence(17)
		if err := params.SetScope("BTC/USD"); err != nil {
			return err
		}
		values, err := params.NewValues(411)
		if err != nil {
			return err
		}
		flags, err := params.NewPresent(411)
		if err != nil {
			return err
		}
		identities, err := params.NewIdentities(411)
		if err != nil {
			return err
		}
		for index := range 411 {
			values.Set(index, float64(index)-205)
			flags.Set(index, index%2 == 0)
			if err := identities.Set(index, fmt.Sprintf("metric:%d", index)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	future, release := client.Done(context.Background(), nil)
	result, err := future.Struct()
	if err != nil {
		t.Fatal(err)
	}
	record, err := result.Row()
	if err != nil {
		t.Fatal(err)
	}
	fields := []arrow.Field{
		{Name: "epoch", Type: arrow.PrimitiveTypes.Int64},
		{Name: "sequence", Type: arrow.PrimitiveTypes.Int64},
		{Name: "symbol", Type: arrow.BinaryTypes.String},
		{Name: "complete", Type: arrow.FixedWidthTypes.Boolean},
		{Name: "metrics", Type: arrow.ListOf(arrow.StructOf(
			arrow.Field{Name: "identity", Type: arrow.BinaryTypes.String},
			arrow.Field{Name: "value", Type: arrow.PrimitiveTypes.Float64},
			arrow.Field{Name: "present", Type: arrow.FixedWidthTypes.Boolean},
			arrow.Field{Name: "epoch", Type: arrow.PrimitiveTypes.Int64},
			arrow.Field{Name: "sequence", Type: arrow.PrimitiveTypes.Int64},
		))},
		{Name: "provenance", Type: arrow.BinaryTypes.String},
	}
	return record, arrow.NewSchema(fields, nil), func() { release(); client.Release() }
}

func TestNativeColumnsAppend(t *testing.T) {
	Convey("Native metric cuts reach Arrow without JSON conversion", t, func() {
		record, schema, release := nativeCutFixture(t)
		defer release()
		builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
		defer builder.Release()
		columns := nativeColumns{}
		for range 3 {
			So(columns.append(builder, record), ShouldBeNil)
		}
		batch := builder.NewRecordBatch()
		defer batch.Release()
		So(batch.NumRows(), ShouldEqual, 3)
		for index := range 3 {
			So(batch.Column(0).(*array.Int64).Value(index), ShouldEqual, int64(9007199254740993))
			So(batch.Column(1).(*array.Int64).Value(index), ShouldEqual, 17)
			So(batch.Column(2).(*array.String).Value(index), ShouldEqual, "BTC/USD")
			So(batch.Column(3).(*array.Boolean).Value(index), ShouldBeFalse)
		}
		metrics := batch.Column(4).(*array.List).ListValues().(*array.Struct)
		So(metrics.Len(), ShouldEqual, 3*411)
		So(metrics.Field(1).(*array.Float64).Value(0), ShouldEqual, -205)
		So(metrics.Field(2).(*array.Boolean).Value(1), ShouldBeFalse)
		So(metrics.Field(3).(*array.Int64).Value(2), ShouldEqual, int64(9007199254740993))
		So(metrics.Field(4).(*array.Int64).Value(2), ShouldEqual, 17)

		Convey("Unknown columns and mismatched native types fail explicitly", func() {
			invalid := array.NewRecordBuilder(memory.DefaultAllocator, arrow.NewSchema([]arrow.Field{{Name: "symbol", Type: arrow.PrimitiveTypes.Float64}}, nil))
			defer invalid.Release()
			changed := nativeColumns{}
			So(changed.append(invalid, record), ShouldNotBeNil)
			missing := array.NewRecordBuilder(memory.DefaultAllocator, arrow.NewSchema([]arrow.Field{{Name: "notAField", Type: arrow.PrimitiveTypes.Float64}}, nil))
			defer missing.Release()
			So(changed.append(missing, record), ShouldNotBeNil)
		})
	})
}

func BenchmarkNativeColumnsAppend(b *testing.B) {
	record, schema, release := nativeCutFixture(b)
	defer release()
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer builder.Release()
	columns := nativeColumns{}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := columns.append(builder, record); err != nil {
			b.Fatal(err)
		}
		batch := builder.NewRecordBatch()
		batch.Release()
		// Each iteration stands for a distinct immutable message with its own traversal budget.
		record.Message().ResetReadLimit(math.MaxUint64)
	}
}
