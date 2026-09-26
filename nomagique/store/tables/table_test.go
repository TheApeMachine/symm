package tables

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/apache/iceberg-go/catalog"
	icetable "github.com/apache/iceberg-go/table"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
	client := store.Capture_ServerToClient(store.NewCapture(context.Background()))
	defer client.Release()
	ctx := context.Background()
	err := client.Write(ctx, func(params store.Capture_write_Params) error {
		provenance, err := params.NewProvenance(1)
		if err != nil {
			return err
		}
		payloads, err := params.NewPayload(1)

		if err != nil {
			return err
		}

		for _, err := range []error{provenance.Set(0, []byte(`{"session":"fixture","sequence":0,"endpoint":"wss://fixture.test/feed","receivedAt":"2026-09-22T12:00:00.123456789Z"}`)), payloads.Set(0, payload)} {
			if err != nil {
				return err
			}
		}
		return nil
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

	out, err := result.Row().Out()

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

		Convey("Rows handed over together are held in order alongside payloads", func() {
			first, second := captureRow(t, []byte(`{"channel":"a"}`)), captureRow(t, []byte(`{"channel":"b"}`))
			So(client.Write(ctx, func(params IcebergTable_write_Params) error {
				if err := params.SetConfig(captureDeclaration); err != nil {
					return err
				}

				rows, err := params.NewRows(3)

				if err != nil {
					return err
				}

				for index, row := range [][]byte{first, nil, second} {
					if err := rows.Set(index, row); err != nil {
						return err
					}
				}

				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			So(writer.pending, ShouldHaveLength, 4)
			So(writer.pending[2].raw, ShouldResemble, first)
			So(writer.pending[3].raw, ShouldResemble, second)
		})

		Convey("A size-triggered append keeps its partial tail until an explicit flush", func() {
			// The first two real rows fill one byte budget; the third is a tail.
			writer.appendBytes = writer.held
			row := captureRow(t, []byte(`{"channel":"tail"}`))
			So(client.Write(ctx, func(params IcebergTable_write_Params) error {
				if err := params.SetConfig(captureDeclaration); err != nil {
					return err
				}
				return params.SetPayload(row)
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			So(writer.commit(ctx, false), ShouldBeNil)
			So(writer.committed, ShouldEqual, 2)
			So(writer.pending, ShouldHaveLength, 1)
			So(writer.pending[0].raw, ShouldResemble, row)
			So(writer.held, ShouldEqual, len(row))
			So(writer.inflight, ShouldEqual, 0)
			So(writer.commit(ctx, true), ShouldBeNil)
			So(writer.committed, ShouldEqual, 3)
			So(writer.pending, ShouldBeEmpty)
			So(writer.held, ShouldEqual, 0)
		})

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
				properties, err := params.NewProperties(1)
				if err != nil {
					return err
				}
				if err := properties.Set(0, []byte(`{}`)); err != nil {
					return err
				}
				arrivals, err := params.NewMetadata(1)
				if err != nil {
					return err
				}
				return arrivals.Set(0, metadata)
			}), ShouldBeNil)
			So(reader.WaitStreaming(), ShouldBeNil)

			// Batches arrive whole; every row of every batch is one record.
			var rows [][]byte

			for {
				future, release := reader.Done(ctx, nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)

				if result.Exhausted() {
					release()
					break
				}

				ipc, err := result.Out()
				So(err, ShouldBeNil)
				projection := data.Arrow_ServerToClient(data.NewArrow())
				So(projection.Write(ctx, func(args data.Arrow_write_Params) error { return args.SetData(ipc) }), ShouldBeNil)
				So(projection.WaitStreaming(), ShouldBeNil)
				projected, releaseProjection := projection.Done(ctx, nil)
				projectedResult, err := projected.Struct()
				So(err, ShouldBeNil)
				list, err := projectedResult.Rows()
				So(err, ShouldBeNil)

				for index := range list.Len() {
					row, err := list.At(index)
					So(err, ShouldBeNil)
					rows = append(rows, bytes.Clone(row))
				}

				releaseProjection()
				projection.Release()
				release()
			}

			So(rows, ShouldHaveLength, len(payloads))

			for index, payload := range payloads {
				row := rows[index]
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
			server.pending = []tableRow{heldRow(make([]byte, 16))}
			server.held = 16

			So(server.worthSending(), ShouldBeFalse)
		})

		Convey("It sends once the rows add up to what an append is sized for", func() {
			server.pending = []tableRow{heldRow(make([]byte, 40)), heldRow(make([]byte, 40))}
			server.held = 80

			So(server.worthSending(), ShouldBeTrue)
		})

		// A caller that knows the run is ending must be able to say so, or
		// the last rows sit in memory and the tape loses its tail.
		Convey("It sends when a caller says now, whatever it holds", func() {
			server.pending = []tableRow{heldRow(make([]byte, 1))}
			server.held = 1
			server.asked = true

			So(server.worthSending(), ShouldBeTrue)
		})

		Convey("An unbounded budget never sends on size alone", func() {
			server.appendBytes = 0
			server.pending = []tableRow{heldRow(make([]byte, 1<<20))}
			server.held = 1 << 20

			So(server.worthSending(), ShouldBeFalse)
		})
	})
}

/* delayedCatalog stalls real table creation without substituting storage results. */
type delayedCatalog struct {
	catalog.Catalog
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	err     error
}

func (fixture *delayedCatalog) CreateNamespace(ctx context.Context, namespace icetable.Identifier, properties iceberg.Properties) error {
	fixture.once.Do(func() { close(fixture.entered) })
	select {
	case <-fixture.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	if fixture.err != nil {
		return fixture.err
	}
	return fixture.Catalog.CreateNamespace(ctx, namespace, properties)
}

func TestIcebergTableDone(t *testing.T) {
	Convey("Slow catalog I/O is isolated behind the table node's bounded admission", t, func() {
		directory := t.TempDir()
		database, err := sql.Open("sqlite3", filepath.Join(directory, "catalog.db"))
		So(err, ShouldBeNil)
		defer func() { So(database.Close(), ShouldBeNil) }()
		backing, err := sqlcat.NewCatalog("test", database, sqlcat.SQLite, iceberg.Properties{"warehouse": "file://" + directory})
		So(err, ShouldBeNil)
		fixture := &delayedCatalog{Catalog: backing, entered: make(chan struct{}), release: make(chan struct{})}
		writer := NewIcebergTable()
		writer.Catalog = fixture
		client := IcebergTable_ServerToClient(writer)
		defer client.Release()
		var unblock sync.Once
		defer unblock.Do(func() { close(fixture.release) })
		row := []byte(`{"value":1}`)
		declaration := `{"namespace":"test","table":"isolated","fields":[{"id":1,"name":"value","type":"long","required":true}],"appendBytes":11,"maxPendingBytes":22}`
		admit := func() error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := client.Write(ctx, func(args IcebergTable_write_Params) error {
				if err := args.SetConfig(declaration); err != nil {
					return err
				}
				return args.SetPayload(row)
			})
			if err != nil {
				return err
			}
			return client.WaitStreaming()
		}
		So(admit(), ShouldBeNil)
		future, release := client.Done(context.Background(), nil)
		_, err = future.Struct()
		release()
		So(err, ShouldBeNil)
		select {
		case <-fixture.entered:
		case <-time.After(time.Second):
			t.Fatal("catalog worker did not start")
		}
		So(admit(), ShouldBeNil)
		Convey("The next batch is rejected intact before exceeding the memory budget", func() {
			So(admit(), ShouldNotBeNil)
			writer.mutex.Lock()
			held, pending := writer.held, len(writer.pending)
			writer.mutex.Unlock()
			So(held, ShouldEqual, 22)
			So(pending, ShouldEqual, 2)
		})
		Convey("Flush joins the worker and persists both admitted rows", func() {
			unblock.Do(func() { close(fixture.release) })
			future, release := client.Flush(context.Background(), nil)
			_, err := future.Struct()
			release()
			So(err, ShouldBeNil)
			So(writer.committed, ShouldEqual, 2)
			So(writer.held, ShouldEqual, 0)
		})
		Convey("A failed catalog call retains all rows and flush can retry", func() {
			fixture.err = errors.New("catalog unavailable")
			unblock.Do(func() { close(fixture.release) })
			future, release := client.Flush(context.Background(), nil)
			_, err := future.Struct()
			release()
			So(err, ShouldNotBeNil)
			So(writer.held, ShouldEqual, 22)
			So(writer.pending, ShouldHaveLength, 2)
			fixture.err = nil
			future, release = client.Flush(context.Background(), nil)
			_, err = future.Struct()
			release()
			So(err, ShouldBeNil)
			So(writer.committed, ShouldEqual, 2)
		})
	})
}

func BenchmarkIcebergTableFlush(b *testing.B) {
	directory := b.TempDir()
	database, err := sql.Open("sqlite3", filepath.Join(directory, "catalog.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			b.Error(err)
		}
	}()
	backing, err := sqlcat.NewCatalog("test", database, sqlcat.SQLite, iceberg.Properties{"warehouse": "file://" + directory})
	if err != nil {
		b.Fatal(err)
	}
	writer := NewIcebergTable()
	writer.Catalog = backing
	client := IcebergTable_ServerToClient(writer)
	defer client.Release()
	row := captureRow(b, []byte(`{"channel":"ticker","data":[{"symbol":"BTC/USD","last":101.5}]}`))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := client.Write(context.Background(), func(args IcebergTable_write_Params) error {
			if err := args.SetConfig(captureDeclaration); err != nil {
				return err
			}
			return args.SetPayload(row)
		}); err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Flush(context.Background(), nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestIcebergTableWriteNative(t *testing.T) {
	Convey("The table capability owns native arrivals after the caller releases them", t, func() {
		record, _, release := nativeCutFixture(t)
		writer := NewIcebergTable()
		client := IcebergTable_ServerToClient(writer)
		defer client.Release()
		declaration := `{"namespace":"test","table":"native","fields":[{"id":1,"name":"epoch","type":"long","required":true}]}`
		So(client.Write(context.Background(), func(params IcebergTable_write_Params) error {
			if err := params.SetConfig(declaration); err != nil {
				return err
			}
			return params.SetRecord(record)
		}), ShouldBeNil)
		So(client.WaitStreaming(), ShouldBeNil)
		pointer, err := record.Value()
		So(err, ShouldBeNil)
		data.MetricCut(pointer.Struct()).SetSequence(999)
		release()
		So(writer.pending, ShouldHaveLength, 1)
		carried, err := writer.pending[0].record.Value()
		So(err, ShouldBeNil)
		So(data.MetricCut(carried.Struct()).Sequence(), ShouldEqual, 17)
		So(writer.held, ShouldEqual, writer.pending[0].size)
		So(writer.pending[0].raw, ShouldBeEmpty)
		writer.pending[0].release()
		writer.pending = nil
		writer.held = 0
	})
}
