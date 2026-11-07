package workbench

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

/*
local opens an in-memory DuckDB with fixture views for Warehouse tests.
*/
func local(t *testing.T) *Warehouse {
	t.Helper()

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}

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

	warehouse := &Warehouse{db: db, catalog: NewCatalog("symm")}
	t.Cleanup(func() { _ = warehouse.Close() })
	return warehouse
}
