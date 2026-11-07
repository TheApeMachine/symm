package workbench

import (
	"fmt"
	"strings"
)

/*
Catalog seeds DuckDB views over S3 JSON prefixes the trading path already writes.
*/
type Catalog struct {
	bucket string
}

/*
seed is one view: prefer the S3-backed definition, fall back to an empty typed
view when the prefix has no objects yet (DuckDB rejects empty globs).
*/
type seed struct {
	name    string
	primary string
	empty   string
}

func NewCatalog(bucket string) *Catalog {
	return &Catalog{bucket: bucket}
}

/*
qualified names views in the attached `warehouse` catalog so Perspective's
WarehouseHandler can keep filtering out its own `memory` working tables.
*/
func qualified(name string) string {
	return "warehouse.main." + identifier(name)
}

func (catalog *Catalog) seeds() []seed {
	out := make([]seed, 0, 10)

	for _, ledger := range []string{"paper", "real", "virtual"} {
		for _, state := range []string{"open", "closed"} {
			name := state + "_" + ledger
			glob := fmt.Sprintf("s3://%s/%s/%s/**", catalog.bucket, ledger, state)

			out = append(out, seed{
				name:    name,
				primary: catalog.lotView(name, glob),
				empty:   catalog.emptyLotView(name),
			})
		}
	}

	framesGlob := fmt.Sprintf("s3://%s/market/level3/**/*.ndjson", catalog.bucket)

	out = append(out, seed{
		name:    "level3_frames",
		primary: catalog.level3FramesView(framesGlob),
		empty:   catalog.emptyLevel3FramesView(),
	})
	out = append(out, seed{
		name:    "level3_events",
		primary: catalog.level3EventsView(framesGlob),
		empty:   catalog.emptyLevel3EventsView(),
	})

	return out
}

func (catalog *Catalog) lotView(name, glob string) string {
	// Explicit columns avoid read_json_auto sampling the whole prefix at CREATE VIEW.
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT %s FROM read_json(%s, union_by_name=true, columns=%s)",
		qualified(name),
		strings.Join(lotProjections(), ", "),
		literal(glob),
		lotColumnMap(),
	)
}

func (catalog *Catalog) emptyLotView(name string) string {
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT %s WHERE false",
		qualified(name),
		strings.Join(emptyLotColumns(), ", "),
	)
}

func lotColumnMap() string {
	return `{` +
		`'epoch': 'UBIGINT', ` +
		`'tick': 'UBIGINT', ` +
		`'timestamp': 'UBIGINT', ` +
		`'position': 'JSON'` +
		`}`
}

func lotProjections() []string {
	return []string{
		"epoch",
		"tick",
		"timestamp",
		"json_extract_string(position, '$.id') AS id",
		"json_extract_string(position, '$.symbol') AS symbol",
		"json_extract_string(position, '$.book') AS book",
		"json_extract_string(position, '$.entry_path') AS entry_path",
		"json_extract_string(position, '$.exit_path') AS exit_path",
		"json_extract_string(position, '$.exit_trigger') AS exit_trigger",
		"json_extract_string(position, '$.qty') AS qty",
		"json_extract_string(position, '$.entry') AS entry_price",
		"json_extract_string(position, '$.exit_vwap') AS exit_price",
		"json_extract_string(position, '$.pnl') AS pnl",
		"CAST(json_extract(position, '$.pnl_pct') AS DOUBLE) AS pnl_pct",
		"json_extract_string(position, '$.opened_at') AS opened_at",
		"json_extract_string(position, '$.closed_at') AS closed_at",
		"json_extract(position, '$.fills')::VARCHAR AS fills",
		"json_extract(position, '$.hold_paths')::VARCHAR AS hold_paths",
	}
}

func emptyLotColumns() []string {
	return []string{
		"CAST(NULL AS UBIGINT) AS epoch",
		"CAST(NULL AS UBIGINT) AS tick",
		"CAST(NULL AS UBIGINT) AS timestamp",
		"CAST(NULL AS VARCHAR) AS id",
		"CAST(NULL AS VARCHAR) AS symbol",
		"CAST(NULL AS VARCHAR) AS book",
		"CAST(NULL AS VARCHAR) AS entry_path",
		"CAST(NULL AS VARCHAR) AS exit_path",
		"CAST(NULL AS VARCHAR) AS exit_trigger",
		"CAST(NULL AS VARCHAR) AS qty",
		"CAST(NULL AS VARCHAR) AS entry_price",
		"CAST(NULL AS VARCHAR) AS exit_price",
		"CAST(NULL AS VARCHAR) AS pnl",
		"CAST(NULL AS DOUBLE) AS pnl_pct",
		"CAST(NULL AS VARCHAR) AS opened_at",
		"CAST(NULL AS VARCHAR) AS closed_at",
		"CAST(NULL AS VARCHAR) AS fills",
		"CAST(NULL AS VARCHAR) AS hold_paths",
	}
}

func (catalog *Catalog) level3FramesView(glob string) string {
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT "+
			"channel, type, data::VARCHAR AS data "+
			"FROM read_json(%s, format='newline_delimited', columns=%s)",
		qualified("level3_frames"),
		literal(glob),
		level3ColumnMap(),
	)
}

func (catalog *Catalog) emptyLevel3FramesView() string {
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT "+
			"CAST(NULL AS VARCHAR) AS channel, "+
			"CAST(NULL AS VARCHAR) AS type, "+
			"CAST(NULL AS VARCHAR) AS data "+
			"WHERE false",
		qualified("level3_frames"),
	)
}

func level3ColumnMap() string {
	return `{` +
		`'channel': 'VARCHAR', ` +
		`'type': 'VARCHAR', ` +
		`'data': 'JSON'` +
		`}`
}

func (catalog *Catalog) level3EventsView(glob string) string {
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT "+
			"f.channel AS channel, "+
			"f.type AS frame_type, "+
			"json_extract_string(e, '$.symbol') AS symbol, "+
			"json_extract_string(e, '$.type') AS type, "+
			"json_extract_string(e, '$.timestamp') AS timestamp, "+
			"TRY_CAST(json_extract(e, '$.checksum') AS UBIGINT) AS checksum, "+
			"json_extract(e, '$.bids')::VARCHAR AS bids, "+
			"json_extract(e, '$.asks')::VARCHAR AS asks "+
			"FROM read_json(%s, format='newline_delimited', columns=%s) AS f, "+
			"UNNEST(CAST(f.data AS JSON[])) AS t(e)",
		qualified("level3_events"),
		literal(glob),
		level3ColumnMap(),
	)
}

func (catalog *Catalog) emptyLevel3EventsView() string {
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT "+
			"CAST(NULL AS VARCHAR) AS channel, "+
			"CAST(NULL AS VARCHAR) AS frame_type, "+
			"CAST(NULL AS VARCHAR) AS symbol, "+
			"CAST(NULL AS VARCHAR) AS type, "+
			"CAST(NULL AS VARCHAR) AS timestamp, "+
			"CAST(NULL AS UBIGINT) AS checksum, "+
			"CAST(NULL AS VARCHAR) AS bids, "+
			"CAST(NULL AS VARCHAR) AS asks "+
			"WHERE false",
		qualified("level3_events"),
	)
}
