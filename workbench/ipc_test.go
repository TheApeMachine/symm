package workbench

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAsInt64(t *testing.T) {
	Convey("Given DuckDB unsigned integers", t, func() {
		Convey("It should keep uint, uint8, uint16, uint32, and uint64", func() {
			So(asInt64(uint(42)), ShouldEqual, 42)
			So(asInt64(uint8(7)), ShouldEqual, 7)
			So(asInt64(uint16(1000)), ShouldEqual, 1000)
			So(asInt64(uint32(100000)), ShouldEqual, 100000)
			So(asInt64(uint64(99)), ShouldEqual, 99)
			So(asInt64(uint64(math.MaxUint64)), ShouldEqual, math.MaxInt64)
		})

		Convey("It should keep signed values", func() {
			So(asInt64(int32(-3)), ShouldEqual, -3)
			So(asInt64(int64(11)), ShouldEqual, 11)
		})
	})
}

func TestEncodeIPC(t *testing.T) {
	Convey("Given typed DuckDB columns and rows", t, func() {
		when := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
		columns := []Column{
			{Name: "sym", Type: "VARCHAR"},
			{Name: "qty", Type: "DOUBLE"},
			{Name: "ok", Type: "BOOL"},
			{Name: "ts", Type: "TIMESTAMP"},
		}
		rows := [][]any{
			{"BTC/USD", 1.5, true, when},
			{"ETH/USD", nil, false, nil},
		}

		buf, err := encodeIPC(columns, rows)
		So(err, ShouldBeNil)

		reader, err := ipc.NewReader(bytes.NewReader(buf))
		So(err, ShouldBeNil)
		defer reader.Release()

		Convey("It round-trips an Arrow IPC stream", func() {
			So(reader.Schema().NumFields(), ShouldEqual, 4)
			So(reader.Next(), ShouldBeTrue)

			rec := reader.Record()
			defer rec.Release()

			So(rec.NumRows(), ShouldEqual, 2)
			So(rec.NumCols(), ShouldEqual, 4)

			symCol := rec.Column(0).(*array.String)
			So(symCol.Value(0), ShouldEqual, "BTC/USD")
			So(symCol.IsNull(1), ShouldBeFalse)

			qtyCol := rec.Column(1).(*array.Float64)
			So(qtyCol.Value(0), ShouldAlmostEqual, 1.5, 0.0001)
			So(qtyCol.IsNull(1), ShouldBeTrue)

			okCol := rec.Column(2).(*array.Boolean)
			So(okCol.Value(0), ShouldBeTrue)
			So(okCol.Value(1), ShouldBeFalse)

			tsCol := rec.Column(3).(*array.Timestamp)
			So(tsCol.IsNull(0), ShouldBeFalse)
			So(arrow.Timestamp(tsCol.Value(0)), ShouldEqual, arrow.Timestamp(when.UnixMicro()))
			So(tsCol.IsNull(1), ShouldBeTrue)

			So(reader.Next(), ShouldBeFalse)
		})
	})
}
