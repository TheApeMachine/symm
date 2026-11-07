package workbench

import (
	"bytes"
	"context"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	_ "github.com/duckdb/duckdb-go/v2"
	. "github.com/smartystreets/goconvey/convey"
)

func TestWarehouseQuery(t *testing.T) {
	Convey("Warehouse returns Arrow IPC for lot and level3 views", t, func() {
		warehouse := local(t)
		ctx := context.Background()

		stream, err := warehouse.Execute(ctx, `SELECT id, symbol, fills FROM closed_paper`)
		So(err, ShouldBeNil)
		So(len(stream), ShouldBeGreaterThan, 0)

		reader, err := ipc.NewReader(bytes.NewReader(stream))
		So(err, ShouldBeNil)
		defer reader.Release()

		So(reader.Next(), ShouldBeTrue)
		record := reader.Record()
		So(record.NumRows(), ShouldEqual, 1)
		So(record.Column(0).(*array.String).Value(0), ShouldEqual, "lot-1")

		events, err := warehouse.Execute(ctx, `SELECT symbol, frame_type FROM level3_events ORDER BY symbol`)
		So(err, ShouldBeNil)

		eventReader, err := ipc.NewReader(bytes.NewReader(events))
		So(err, ShouldBeNil)
		defer eventReader.Release()

		So(eventReader.Next(), ShouldBeTrue)
		So(eventReader.Record().NumRows(), ShouldEqual, 2)
	})
}

func TestWarehouseExecuteSession(t *testing.T) {
	Convey("Warehouse materializes and drops views with an empty Arrow body", t, func() {
		warehouse := local(t)
		ctx := context.Background()

		empty, err := warehouse.Execute(ctx, `CREATE OR REPLACE VIEW v1 AS SELECT id FROM closed_paper`)
		So(err, ShouldBeNil)
		So(len(empty), ShouldEqual, 0)

		stream, err := warehouse.Execute(ctx, `SELECT id FROM v1`)
		So(err, ShouldBeNil)
		So(len(stream), ShouldBeGreaterThan, 0)

		dropped, err := warehouse.Execute(ctx, `DROP VIEW v1`)
		So(err, ShouldBeNil)
		So(len(dropped), ShouldEqual, 0)
	})

	Convey("Warehouse materializes Perspective working tables", t, func() {
		warehouse := local(t)
		ctx := context.Background()

		empty, err := warehouse.Execute(ctx,
			`CREATE TABLE snap AS SELECT id, symbol FROM closed_paper`)
		So(err, ShouldBeNil)
		So(len(empty), ShouldEqual, 0)

		stream, err := warehouse.Execute(ctx, `SELECT id FROM snap`)
		So(err, ShouldBeNil)
		So(len(stream), ShouldBeGreaterThan, 0)

		dropped, err := warehouse.Execute(ctx, `DROP TABLE snap`)
		So(err, ShouldBeNil)
		So(len(dropped), ShouldEqual, 0)
	})
}
