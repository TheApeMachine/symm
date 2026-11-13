package workbench

import (
	"bytes"
	"context"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	. "github.com/smartystreets/goconvey/convey"
)

func TestExecute(t *testing.T) {
	Convey("Setup", t, func() {
		warehouse := local(t)
		ctx := context.Background()

		Convey("When Execute runs a SELECT over fixture views", func() {
			stream, err := warehouse.Execute(ctx, `SELECT id, symbol, fills FROM closed_paper`)

			Convey("Arrow IPC carries the closed position row", func() {
				So(err, ShouldBeNil)
				So(len(stream), ShouldEqual, 568)

				reader, err := ipc.NewReader(bytes.NewReader(stream))
				So(err, ShouldBeNil)
				defer reader.Release()

				So(reader.Next(), ShouldBeTrue)
				record := reader.Record()
				So(record.NumRows(), ShouldEqual, 1)
				So(record.Column(0).(*array.String).Value(0), ShouldEqual, "position-1")
			})
		})

		Convey("When Execute runs a SELECT over level3 events", func() {
			events, err := warehouse.Execute(ctx, `SELECT symbol, frame_type FROM level3_events ORDER BY symbol`)

			Convey("Arrow IPC carries both fixture symbols", func() {
				So(err, ShouldBeNil)

				eventReader, err := ipc.NewReader(bytes.NewReader(events))
				So(err, ShouldBeNil)
				defer eventReader.Release()

				So(eventReader.Next(), ShouldBeTrue)
				So(eventReader.Record().NumRows(), ShouldEqual, 2)
			})
		})

		Convey("When Execute runs CREATE OR REPLACE VIEW", func() {
			empty, err := warehouse.Execute(ctx, `CREATE OR REPLACE VIEW v1 AS SELECT id FROM closed_paper`)

			Convey("The body is empty", func() {
				So(err, ShouldBeNil)
				So(len(empty), ShouldEqual, 0)
			})
		})

		Convey("When a session view is queried then dropped", func() {
			_, err := warehouse.Execute(ctx, `CREATE OR REPLACE VIEW v1 AS SELECT id FROM closed_paper`)
			So(err, ShouldBeNil)

			stream, err := warehouse.Execute(ctx, `SELECT id FROM v1`)
			So(err, ShouldBeNil)
			So(len(stream), ShouldEqual, 312)

			dropped, err := warehouse.Execute(ctx, `DROP VIEW v1`)

			Convey("The drop returns an empty body", func() {
				So(err, ShouldBeNil)
				So(len(dropped), ShouldEqual, 0)
			})
		})

		Convey("When Execute materializes a working table", func() {
			empty, err := warehouse.Execute(ctx,
				`CREATE TABLE snap AS SELECT id, symbol FROM closed_paper`)

			Convey("CREATE TABLE returns empty then SELECT streams rows", func() {
				So(err, ShouldBeNil)
				So(len(empty), ShouldEqual, 0)

				stream, err := warehouse.Execute(ctx, `SELECT id FROM snap`)
				So(err, ShouldBeNil)
				So(len(stream), ShouldEqual, 312)

				dropped, err := warehouse.Execute(ctx, `DROP TABLE snap`)
				So(err, ShouldBeNil)
				So(len(dropped), ShouldEqual, 0)
			})
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Setup", t, func() {
		warehouse := New()

		Convey("When Close is called without a prior connect", func() {
			err := warehouse.Close()

			Convey("It succeeds without error", func() {
				So(err, ShouldBeNil)
			})
		})
	})
}

func BenchmarkExecute(b *testing.B) {
	warehouse := local(b)
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		_, _ = warehouse.Execute(ctx, `SELECT id, symbol FROM closed_paper`)
	}
}

func BenchmarkClose(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		warehouse := New()
		_ = warehouse.Close()
	}
}
