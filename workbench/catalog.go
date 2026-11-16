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
	out := make([]seed, 0, 13)

	for _, ledger := range []string{"paper", "real", "virtual"} {
		for _, state := range []string{"open", "closed"} {
			name := state + "_" + ledger
			glob := fmt.Sprintf("s3://%s/%s/%s/**", catalog.bucket, ledger, state)

			out = append(out, seed{
				name:    name,
				primary: catalog.positionView(name, glob),
				empty:   catalog.emptyPositionView(name),
			})
		}
	}

	for _, ledger := range []string{"paper", "real", "virtual"} {
		name := "stoploss_" + ledger
		glob := fmt.Sprintf("s3://%s/%s/stops/**", catalog.bucket, ledger)

		out = append(out, seed{
			name:    name,
			primary: catalog.stoplossView(name, glob),
			empty:   catalog.emptyStoplossView(name),
		})
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

func (catalog *Catalog) positionView(name, glob string) string {
	// Explicit columns avoid read_json_auto sampling the whole prefix at CREATE VIEW.
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT %s FROM read_json(%s, union_by_name=true, columns=%s)",
		qualified(name),
		strings.Join(positionProjections(), ", "),
		literal(glob),
		positionColumnMap(),
	)
}

func (catalog *Catalog) emptyPositionView(name string) string {
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT %s WHERE false",
		qualified(name),
		strings.Join(emptyPositionColumns(), ", "),
	)
}

func positionColumnMap() string {
	return `{` +
		`'epoch': 'UBIGINT', ` +
		`'tick': 'UBIGINT', ` +
		`'timestamp': 'UBIGINT', ` +
		`'position': 'JSON'` +
		`}`
}

func positionProjections() []string {
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

func emptyPositionColumns() []string {
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

/*
stoplossView reads the temporal StopHistory tape: one Stoploss JSON per
lifecycle beat under {ledger}/stops/{symbol}/{positionID}/{nanos}. Path parts
come from filename so symbol slashes (BTC/USD) stay intact.
*/
func (catalog *Catalog) stoplossView(name, glob string) string {
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT %s FROM read_json(%s, filename=true, union_by_name=true, columns=%s)",
		qualified(name),
		strings.Join(stoplossProjections(), ", "),
		literal(glob),
		stoplossColumnMap(),
	)
}

func (catalog *Catalog) emptyStoplossView(name string) string {
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT %s WHERE false",
		qualified(name),
		strings.Join(emptyStoplossColumns(), ", "),
	)
}

func stoplossColumnMap() string {
	return `{` +
		`'State': 'INTEGER', ` +
		`'Regime': 'VARCHAR', ` +
		`'InitialPnLPct': 'DOUBLE', ` +
		`'CurrentPnLPct': 'DOUBLE', ` +
		`'PeakPnLPct': 'DOUBLE', ` +
		`'ArmPnlPct': 'DOUBLE', ` +
		`'TrailingFloor': 'DOUBLE', ` +
		`'ExhaustScore': 'DOUBLE', ` +
		`'Confirms': 'INTEGER', ` +
		`'TypicalPush': 'DOUBLE', ` +
		`'TypicalPullback': 'DOUBLE', ` +
		`'CurrentDrop': 'DOUBLE', ` +
		`'EmaDelta': 'DOUBLE', ` +
		`'TriggerReason': 'VARCHAR', ` +
		`'MaxWindow': 'INTEGER', ` +
		`'MaxSwings': 'INTEGER', ` +
		`'RallyStart': 'DOUBLE', ` +
		`'PullbackLow': 'DOUBLE', ` +
		`'PushRecorded': 'BOOLEAN', ` +
		`'EmaInit': 'BOOLEAN', ` +
		`'FlatTicks': 'INTEGER', ` +
		`'ExplosiveActive': 'BOOLEAN', ` +
		`'ExplosivePushes': 'INTEGER', ` +
		`'ExplosiveTTL': 'INTEGER', ` +
		`'PeakPushDelta': 'DOUBLE'` +
		`}`
}

func stoplossProjections() []string {
	path := `regexp_extract(filename, '/stops/(.+)/([^/]+)/([0-9]{19})$'`

	return []string{
		path + `, 1) AS symbol`,
		path + `, 2) AS position_id`,
		`TRY_CAST(` + path + `, 3) AS UBIGINT) AS noted_at_ns`,
		`State AS state`,
		`Regime AS regime`,
		`InitialPnLPct AS initial_pnl_pct`,
		`CurrentPnLPct AS current_pnl_pct`,
		`PeakPnLPct AS peak_pnl_pct`,
		`ArmPnlPct AS arm_pnl_pct`,
		`TrailingFloor AS trailing_floor`,
		`ExhaustScore AS exhaust_score`,
		`Confirms AS confirms`,
		`TypicalPush AS typical_push`,
		`TypicalPullback AS typical_pullback`,
		`CurrentDrop AS current_drop`,
		`EmaDelta AS ema_delta`,
		`TriggerReason AS trigger_reason`,
		`(PeakPnLPct - CurrentPnLPct) AS giveback_pct`,
		`ExplosiveActive AS explosive_active`,
		`ExplosivePushes AS explosive_pushes`,
		`PeakPushDelta AS peak_push_delta`,
	}
}

func emptyStoplossColumns() []string {
	return []string{
		"CAST(NULL AS VARCHAR) AS symbol",
		"CAST(NULL AS VARCHAR) AS position_id",
		"CAST(NULL AS UBIGINT) AS noted_at_ns",
		"CAST(NULL AS INTEGER) AS state",
		"CAST(NULL AS VARCHAR) AS regime",
		"CAST(NULL AS DOUBLE) AS initial_pnl_pct",
		"CAST(NULL AS DOUBLE) AS current_pnl_pct",
		"CAST(NULL AS DOUBLE) AS peak_pnl_pct",
		"CAST(NULL AS DOUBLE) AS arm_pnl_pct",
		"CAST(NULL AS DOUBLE) AS trailing_floor",
		"CAST(NULL AS DOUBLE) AS exhaust_score",
		"CAST(NULL AS INTEGER) AS confirms",
		"CAST(NULL AS DOUBLE) AS typical_push",
		"CAST(NULL AS DOUBLE) AS typical_pullback",
		"CAST(NULL AS DOUBLE) AS current_drop",
		"CAST(NULL AS DOUBLE) AS ema_delta",
		"CAST(NULL AS VARCHAR) AS trigger_reason",
		"CAST(NULL AS DOUBLE) AS giveback_pct",
		"CAST(NULL AS BOOLEAN) AS explosive_active",
		"CAST(NULL AS INTEGER) AS explosive_pushes",
		"CAST(NULL AS DOUBLE) AS peak_push_delta",
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
