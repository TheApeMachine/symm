package tables

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/apache/iceberg-go"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
)

const captureDeclaration = `{"namespace":"test","table":"frames","fields":[{"id":1,"name":"received_at","type":"timestamp","required":true},{"id":2,"name":"endpoint","type":"string","required":true},{"id":3,"name":"symbol","type":"string"},{"id":4,"name":"kind","type":"string"},{"id":5,"name":"payload","type":"binary","required":true},{"id":6,"name":"capture_id","type":"string","required":true},{"id":7,"name":"capture_session","type":"string","required":true},{"id":8,"name":"capture_sequence","type":"long","required":true},{"id":9,"name":"received_time","type":"string","required":true}]}`

/* captureRow exercises the actual capture capability, not a hand-built envelope. */
func captureRow(t testing.TB, payload []byte) []byte {
	t.Helper()
	client := store.Capture_ServerToClient(store.NewCapture())
	defer client.Release()
	ctx := context.Background()
	err := client.Write(ctx, func(params store.Capture_write_Params) error {
		if err := params.SetEndpoint("wss://fixture.test/feed"); err != nil {
			return err
		}

		if err := params.SetReceivedAt("2026-09-22T12:00:00.123456789Z"); err != nil {
			return err
		}

		return params.SetPayload(payload)
	})

	if err != nil {
		t.Fatal(err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatal(err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	result, err := future.Struct()

	if err != nil {
		t.Fatal(err)
	}

	out, err := result.Out()

	if err != nil {
		t.Fatal(err)
	}

	return bytes.Clone(out)
}

func TestIcebergTableFlush(t *testing.T) {
	Convey("Given an isolated real Iceberg catalog and raw capture", t, func() {
		ctx := context.Background()
		directory := t.TempDir()
		database, err := sql.Open("sqlite3", filepath.Join(directory, "catalog.db"))
		So(err, ShouldBeNil)
		t.Cleanup(func() {
			if err := database.Close(); err != nil {
				t.Error(err)
			}
		})
		catalog, err := sqlcat.NewCatalog("test", database, sqlcat.SQLite, iceberg.Properties{"warehouse": "file://" + directory})
		So(err, ShouldBeNil)
		So(catalog.CreateNamespace(ctx, []string{"test"}, nil), ShouldBeNil)
		writer := NewIcebergTable()
		writer.Catalog = catalog
		client := IcebergTable_ServerToClient(writer)
		defer client.Release()
		payloads := [][]byte{[]byte(" {\"channel\":\"ticker\", \"data\":[{\"last\":123}]}\n"), {0, 1, 255, 2}}

		for _, payload := range payloads {
			row := captureRow(t, payload)
			So(client.Write(ctx, func(params IcebergTable_write_Params) error {
				if err := params.SetConfig(captureDeclaration); err != nil {
					return err
				}

				return params.SetPayload(row)
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
		}
		So(writer.pending, ShouldHaveLength, 2)

		Convey("Lifecycle flush persists the below-budget tail and scan returns exact bytes", func() {
			future, release := runtime.Durable(client).Flush(ctx, nil)
			_, err := future.Struct()
			release()
			So(err, ShouldBeNil)
			So(writer.pending, ShouldBeEmpty)
			So(writer.committed, ShouldEqual, 2)
			scanner := NewIcebergScan()
			metadata, err := json.Marshal(writer.table.Metadata())
			So(err, ShouldBeNil)
			reader := IcebergScan_ServerToClient(scanner)
			defer reader.Release()
			So(reader.Write(ctx, func(params IcebergScan_write_Params) error {
				if err := params.SetProperties([]byte(`{}`)); err != nil {
					return err
				}
				arrivals, err := params.NewMetadata(1)
				if err != nil {
					return err
				}
				return arrivals.Set(0, metadata)
			}), ShouldBeNil)
			So(reader.WaitStreaming(), ShouldBeNil)

			for _, payload := range payloads {
				future, release := reader.Done(ctx, nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)
				ipc, err := result.Out()
				So(err, ShouldBeNil)
				projection := data.Arrow_ServerToClient(data.NewArrow())
				So(projection.Write(ctx, func(args data.Arrow_write_Params) error { return args.SetData(ipc) }), ShouldBeNil)
				So(projection.WaitStreaming(), ShouldBeNil)
				projected, releaseProjection := projection.Done(ctx, nil)
				projectedResult, err := projected.Struct()
				So(err, ShouldBeNil)
				row, err := projectedResult.Out()
				So(err, ShouldBeNil)
				defer releaseProjection()
				defer projection.Release()
				var record store.CaptureRecord
				So(json.Unmarshal(row, &record), ShouldBeNil)
				frame := record.Payload
				So(record.ReceivedTime, ShouldEqual, "2026-09-22T12:00:00.123456789Z")
				So(record.Session, ShouldNotBeEmpty)
				So(frame, ShouldResemble, payload)
				if json.Valid(frame) {
					grid := store.Grid_ServerToClient(store.NewGrid(ctx))
					So(grid.Write(ctx, func(params store.Grid_write_Params) error {
						if err := params.SetInterests("data.0.last"); err != nil {
							return err
						}

						data, err := params.NewData(1)

						if err != nil {
							return err
						}

						return data.Set(0, frame)
					}), ShouldBeNil)
					So(grid.WaitStreaming(), ShouldBeNil)
					future, releaseGrid := grid.Done(ctx, nil)
					reading, err := future.Struct()
					So(err, ShouldBeNil)
					values, err := reading.Values()
					So(err, ShouldBeNil)
					present, err := reading.Present()
					So(err, ShouldBeNil)
					So(present.At(0), ShouldBeTrue)
					So(values.At(0), ShouldEqual, 123)
					releaseGrid()
					grid.Release()
				}
				release()
			}
		})
	})
}

/*
Every append is a snapshot plus a metadata write. Committing on every
evaluation makes one snapshot per observation and turns the catalog into the
clock, which is what the byte budget and the explicit signal exist to stop.
*/
func TestWorthSending(t *testing.T) {
	Convey("Given a writer holding rows", t, func() {
		server := NewIcebergTable()
		server.appendBytes = 64

		Convey("It waits while what it holds is not worth a snapshot", func() {
			server.pending = [][]byte{make([]byte, 16)}
			server.held = 16

			So(server.worthSending(), ShouldBeFalse)
		})

		Convey("It sends once the rows add up to what an append is sized for", func() {
			server.pending = [][]byte{make([]byte, 40), make([]byte, 40)}
			server.held = 80

			So(server.worthSending(), ShouldBeTrue)
		})

		// A caller that knows the run is ending must be able to say so, or
		// the last rows sit in memory and the tape loses its tail.
		Convey("It sends when a caller says now, whatever it holds", func() {
			server.pending = [][]byte{make([]byte, 1)}
			server.held = 1
			server.asked = true

			So(server.worthSending(), ShouldBeTrue)
		})

		Convey("An unbounded budget never sends on size alone", func() {
			server.appendBytes = 0
			server.pending = [][]byte{make([]byte, 1<<20)}
			server.held = 1 << 20

			So(server.worthSending(), ShouldBeFalse)
		})
	})
}
