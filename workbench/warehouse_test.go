package workbench_test

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	_ "github.com/duckdb/duckdb-go/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/workbench"
)

func local(t *testing.T) *workbench.Warehouse {
	t.Helper()

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	dir := t.TempDir()
	lot := filepath.Join(dir, "lot.json")
	frames := filepath.Join(dir, "frames.ndjson")

	if err := os.WriteFile(lot, []byte(`{
		"epoch": 1,
		"tick": 2,
		"timestamp": 3,
		"position": {
			"id": "lot-1",
			"symbol": "BTC/USD",
			"book": "paper",
			"virtual": false,
			"qty": "1.5",
			"pnl_pct": 0.25,
			"fills": [{"side":"buy"}],
			"hold_paths": ["a/b"]
		}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(frames, []byte(
		`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","type":"update","checksum":1,"bids":[{"order_id":"1"}],"asks":[]}]}`+"\n"+
			`{"channel":"level3","type":"snapshot","data":[{"symbol":"ETH/USD","type":"snapshot","checksum":2,"bids":[],"asks":[{"order_id":"2"}]}]}`+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	lotURL := filepath.ToSlash(lot)
	framesURL := filepath.ToSlash(frames)

	statements := []string{
		`CREATE OR REPLACE VIEW closed_paper AS SELECT
			epoch, tick, timestamp,
			json_extract_string(position, '$.id') AS id,
			json_extract_string(position, '$.symbol') AS symbol,
			json_extract_string(position, '$.book') AS book,
			json_extract(position, '$.fills')::VARCHAR AS fills,
			json_extract(position, '$.hold_paths')::VARCHAR AS hold_paths
		FROM read_json('` + lotURL + `', columns={'epoch':'UBIGINT','tick':'UBIGINT','timestamp':'UBIGINT','position':'JSON'})`,
		`CREATE OR REPLACE VIEW level3_frames AS SELECT
			channel, type, data::VARCHAR AS data
		FROM read_json('` + framesURL + `', format='newline_delimited', columns={'channel':'VARCHAR','type':'VARCHAR','data':'JSON'})`,
		`CREATE OR REPLACE VIEW level3_events AS SELECT
			f.channel AS channel,
			f.type AS frame_type,
			json_extract_string(e, '$.symbol') AS symbol,
			json_extract_string(e, '$.type') AS type,
			TRY_CAST(json_extract(e, '$.checksum') AS UBIGINT) AS checksum,
			json_extract(e, '$.bids')::VARCHAR AS bids,
			json_extract(e, '$.asks')::VARCHAR AS asks
		FROM read_json('` + framesURL + `', format='newline_delimited', columns={'channel':'VARCHAR','type':'VARCHAR','data':'JSON'}) AS f,
		UNNEST(CAST(f.data AS JSON[])) AS t(e)`,
	}

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}

	warehouse := workbench.Wrap(db)
	t.Cleanup(func() { _ = warehouse.Close() })
	return warehouse
}

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
}
