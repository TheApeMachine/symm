package tables

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/iceberg-go"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
)

const captureDeclaration = `{"namespace":"test","table":"frames","fields":[{"id":1,"name":"received_at","type":"timestamp","required":true},{"id":2,"name":"endpoint","type":"string","required":true},{"id":3,"name":"symbol","type":"string"},{"id":4,"name":"kind","type":"string"},{"id":5,"name":"payload","type":"binary","required":true},{"id":6,"name":"capture_id","type":"string","required":true}]}`

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
		writer.catalog = Wrap(catalog)
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
			scanner.catalog = writer.catalog
			reader := IcebergScan_ServerToClient(scanner)
			defer reader.Release()
			So(reader.Write(ctx, func(params IcebergScan_write_Params) error { return params.SetConfig(captureDeclaration) }), ShouldBeNil)
			So(reader.WaitStreaming(), ShouldBeNil)

			for _, payload := range payloads {
				future, release := reader.Done(ctx, nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)
				frame, err := result.Out()
				So(err, ShouldBeNil)
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

func TestFillRow(t *testing.T) {
	Convey("Given a declared capture row", t, func() {
		schema, err := SchemaFromJSON(captureDeclaration)
		So(err, ShouldBeNil)
		converted, err := arrowSchemaFor(schema)
		So(err, ShouldBeNil)
		builder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
		defer builder.Release()
		declared, err := ConfigFromJSON(captureDeclaration)
		So(err, ShouldBeNil)
		raw := []byte(`{"channel":"ticker"}`)
		row := captureRow(t, raw)
		So(fillRow(builder, declared, row), ShouldBeNil)
		record := builder.NewRecordBatch()
		defer record.Release()
		So(record.Column(4).(*array.Binary).Value(0), ShouldResemble, raw)
		So(record.Column(2).IsNull(0), ShouldBeTrue)

		Convey("Missing required fields and malformed binary are explicit errors", func() {
			So(fillRow(builder, declared, raw), ShouldNotBeNil)
			var malformed map[string]any
			So(json.Unmarshal(row, &malformed), ShouldBeNil)
			malformed["payload"] = "not base64!"
			encoded, err := json.Marshal(malformed)
			So(err, ShouldBeNil)
			So(fillRow(builder, declared, encoded), ShouldNotBeNil)
		})
	})
}

func BenchmarkFillRow(b *testing.B) {
	row := captureRow(b, []byte(`{"channel":"ticker","data":[{"last":123.45,"symbol":"BTC/USD"}]}`))
	declared, err := ConfigFromJSON(captureDeclaration)

	if err != nil {
		b.Fatal(err)
	}

	schema, err := SchemaFromJSON(captureDeclaration)

	if err != nil {
		b.Fatal(err)
	}

	converted, err := arrowSchemaFor(schema)

	if err != nil {
		b.Fatal(err)
	}

	builder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
	defer builder.Release()
	b.ReportAllocs()

	for b.Loop() {
		if err := fillRow(builder, declared, row); err != nil {
			b.Fatal(err)
		}

		record := builder.NewRecordBatch()
		record.Release()
	}
}
