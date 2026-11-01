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

func NewCatalog(bucket string) *Catalog {
	return &Catalog{bucket: bucket}
}

func (catalog *Catalog) statements() []string {
	out := make([]string, 0, 10)

	for _, ledger := range []string{"paper", "real", "virtual"} {
		for _, state := range []string{"open", "closed"} {
			name := state + "_" + ledger
			glob := fmt.Sprintf("s3://%s/%s/%s/**", catalog.bucket, ledger, state)

			out = append(out, catalog.lotView(name, glob))
		}
	}

	framesGlob := fmt.Sprintf("s3://%s/market/level3/**/*.ndjson", catalog.bucket)

	out = append(out, catalog.level3FramesView(framesGlob))
	out = append(out, catalog.level3EventsView(framesGlob))

	return out
}

func (catalog *Catalog) lotView(name, glob string) string {
	columns := []string{
		"epoch",
		"tick",
		"timestamp",
		"position.id AS id",
		"position.symbol AS symbol",
		"position.book AS book",
		"position.virtual AS virtual",
		"position.entry_path AS entry_path",
		"position.exit_path AS exit_path",
		"position.exit_trigger AS exit_trigger",
		"position.qty AS qty",
		"position.entry_price AS entry_price",
		"position.exit_price AS exit_price",
		"position.entry_fee AS entry_fee",
		"position.exit_fee AS exit_fee",
		"position.pnl AS pnl",
		"position.pnl_pct AS pnl_pct",
		"position.open_pnl AS open_pnl",
		"position.realized_pct AS realized_pct",
		"position.opened_at AS opened_at",
		"position.closed_at AS closed_at",
		"to_json(position.fills)::VARCHAR AS fills",
		"to_json(position.hold_paths)::VARCHAR AS hold_paths",
		"to_json(position.structural)::VARCHAR AS structural",
		"to_json(position.learning)::VARCHAR AS learning",
	}

	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT %s FROM read_json_auto(%s, union_by_name=true)",
		identifier(name),
		strings.Join(columns, ", "),
		literal(glob),
	)
}

func (catalog *Catalog) level3FramesView(glob string) string {
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT "+
			"channel, type, to_json(data)::VARCHAR AS data "+
			"FROM read_json_auto(%s, format='newline_delimited', union_by_name=true)",
		identifier("level3_frames"),
		literal(glob),
	)
}

func (catalog *Catalog) level3EventsView(glob string) string {
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT "+
			"f.channel AS channel, "+
			"f.type AS frame_type, "+
			"e.symbol AS symbol, "+
			"e.type AS type, "+
			"e.timestamp AS timestamp, "+
			"e.checksum AS checksum, "+
			"to_json(e.bids)::VARCHAR AS bids, "+
			"to_json(e.asks)::VARCHAR AS asks "+
			"FROM read_json_auto(%s, format='newline_delimited', union_by_name=true) AS f, "+
			"UNNEST(f.data) AS t(e)",
		identifier("level3_events"),
		literal(glob),
	)
}
