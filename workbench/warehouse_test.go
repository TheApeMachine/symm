package workbench

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

func TestCatalogName(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"s3://symmtables/":   "symmtables",
		"s3://symmtables":    "symmtables",
		"s3://bucket/path/x": "bucket",
		"symmtables":         "symmtables",
	}

	for location, want := range cases {
		if got := catalogName(location); got != want {
			t.Fatalf("catalogName(%q) = %q, want %q", location, got, want)
		}
	}
}

func TestSanitizeStatement(t *testing.T) {
	t.Parallel()

	got := sanitizeStatement("SELECT FROM memory.main.v1 LIMIT 10")
	want := "SELECT NULL FROM memory.main.v1 LIMIT 10"

	if got != want {
		t.Fatalf("sanitizeStatement = %q, want %q", got, want)
	}

	unchanged := "SELECT 1 FROM dual"
	if sanitizeStatement(unchanged) != unchanged {
		t.Fatalf("sanitizeStatement mutated a valid select")
	}

	tableAs := "CREATE TABLE memory.view_1 AS (SELECT * FROM tape)"
	wantView := "CREATE OR REPLACE VIEW memory.view_1 AS (SELECT * FROM tape)"
	if got := sanitizeStatement(tableAs); got != wantView {
		t.Fatalf("sanitizeStatement = %q, want %q", got, wantView)
	}

	dropTable := "DROP TABLE memory.view_1"
	wantDrop := "DROP VIEW memory.view_1"
	if got := sanitizeStatement(dropTable); got != wantDrop {
		t.Fatalf("sanitizeStatement = %q, want %q", got, wantDrop)
	}
}

func TestNested(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"MAP(VARCHAR, DOUBLE)", "STRUCT(a INTEGER)", "UNION(a INTEGER)", "LIST(INTEGER)", "VARCHAR[]", "INTEGER[]"} {
		if !nested(kind) {
			t.Fatalf("expected nested(%q)", kind)
		}
	}

	for _, kind := range []string{"VARCHAR", "DOUBLE", "BIGINT", "TIMESTAMP WITH TIME ZONE", "BOOLEAN"} {
		if nested(kind) {
			t.Fatalf("did not expect nested(%q)", kind)
		}
	}
}

func TestSelection(t *testing.T) {
	t.Parallel()

	if got := selection("mid", "DOUBLE"); got != `"mid"` {
		t.Fatalf("selection plain = %q", got)
	}

	got := selection("metrics", "MAP(VARCHAR, DOUBLE)")
	want := `to_json("metrics")::VARCHAR AS "metrics"`
	if got != want {
		t.Fatalf("selection nested = %q, want %q", got, want)
	}
}

func TestLiteralAndIdentifier(t *testing.T) {
	t.Parallel()

	if got := literal("a'b"); got != "'a''b'" {
		t.Fatalf("literal = %q", got)
	}

	if got := identifier(`a"b`); got != `"a""b"` {
		t.Fatalf("identifier = %q", got)
	}
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	warehouse := New()
	if warehouse == nil {
		t.Fatal("New returned nil")
	}

	if err := warehouse.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestWarehouse_ConnectAndQuery(t *testing.T) {
	viper.SetConfigFile("../cmd/cfg/config.yml")
	if err := viper.ReadInConfig(); err != nil {
		t.Skipf("Skipping live Iceberg test, could not read config: %v", err)
	}
	system.NewConfig()

	warehouse := New()
	defer warehouse.Close()

	ctx := t.Context()
	db, err := warehouse.DB(ctx)
	if err != nil {
		t.Skipf("Skipping live Iceberg test: %v", err)
	}

	row := db.QueryRowContext(ctx, "SELECT count(*) FROM symmwarehouse.hindsight.measurements")
	var count int64
	if err := row.Scan(&count); err != nil {
		t.Skipf("Skipping live Iceberg test, query failed: %v", err)
		return
	}
	t.Logf("Total measurements via DuckDB: %d", count)

	stream, err := warehouse.Execute(ctx, "SHOW ALL TABLES")
	if err != nil {
		t.Fatalf("warehouse.Execute(SHOW ALL TABLES) failed: %v", err)
	}
	if len(stream) == 0 {
		t.Fatal("expected non-empty arrow ipc stream from SHOW ALL TABLES")
	}

	describeStream, err := warehouse.Execute(ctx, "DESCRIBE symmwarehouse.hindsight.runs")
	if err != nil {
		t.Fatalf("warehouse.Execute(DESCRIBE) failed: %v", err)
	}
	if len(describeStream) == 0 {
		t.Fatal("expected non-empty arrow ipc stream from DESCRIBE")
	}

}

func TestDuckDBSchema(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
		ATTACH ':memory:' AS symmwarehouse;
		CREATE SCHEMA symmwarehouse.hindsight;
		CREATE TABLE symmwarehouse.hindsight.measurements (
			id UINTEGER,
			epoch BIGINT,
			seq_idx BIGINT,
			tick BIGINT,
			"timestamp" BIGINT,
			"at" TIMESTAMP,
			"from" TIMESTAMP,
			label VARCHAR,
			source VARCHAR,
			metrics MAP(VARCHAR, STRUCT(raw DOUBLE, snm DOUBLE, exact VARCHAR, unit VARCHAR, timescale VARCHAR)),
			meta MAP(VARCHAR, VARCHAR)
		);
		INSERT INTO symmwarehouse.hindsight.measurements VALUES (
			1, 100, 1, 10, 1000, '2026-10-14 12:00:00', '2026-10-14 12:00:00',
			'ADA/USD', 'spot:trade',
			map(['mid', 'qty'], [{'raw': 0.75, 'snm': 0.0, 'exact': '0.75', 'unit': 'USD', 'timescale': 'tick'}, {'raw': 10.0, 'snm': 0.0, 'exact': '10.0', 'unit': 'ADA', 'timescale': 'tick'}]),
			map(['tag'], ['test'])
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	var mid float64
	err = db.QueryRow(`SELECT metrics['mid'].raw FROM symmwarehouse.hindsight.measurements`).Scan(&mid)
	if err != nil {
		t.Fatalf("QueryRow metrics['mid'].raw failed: %v", err)
	}
	t.Logf("mid = %f", mid)

	rows, err := db.Query(`SHOW ALL TABLES`)

	if err != nil {
		t.Fatal(err)
	}

	defer rows.Close()

	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}

	cols, _ := rows.Columns()
	t.Logf("SHOW ALL TABLES columns: %v", cols)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		t.Logf("Row: %v", vals)
	}

	ctx := t.Context()
	s3Client := store.NewClient(ctx, "symm")
	if s3Client == nil {
		t.Fatal("store.NewClient is nil")
	}

	start := time.Now()
	imported := 0

	batchSize := 200
	var batchRows []string

	flush := func() error {
		if len(batchRows) == 0 {
			return nil
		}
		query := "INSERT INTO symmwarehouse.hindsight.measurements VALUES " + strings.Join(batchRows, ",")
		_, err := db.ExecContext(ctx, query)
		batchRows = batchRows[:0]
		return err
	}

	for val := range store.Values(s3Client, "") {
		imported++

		metricsMap := "map([], [])"

		var metaKeys []string
		var metaVals []string
		metaKeys = append(metaKeys, literal("origin"), literal("role"), literal("scope"), literal("prefix"))
		metaVals = append(metaVals, literal(val.Origin), literal(val.Role), literal(val.Scope), literal(val.Prefix))
		metaMap := fmt.Sprintf("map([%s], [%s])", strings.Join(metaKeys, ", "), strings.Join(metaVals, ", "))

		at := time.Now()
		if val.Timestamp > 0 {
			at = time.Unix(0, int64(val.Timestamp))
		}
		id := binary.BigEndian.Uint32(val.ID[:4])

		rowStr := fmt.Sprintf(
			"(%d, %d, %d, %d, %d, %s, %s, %s, %s, %s, %s)",
			id, val.Epoch, val.SeqIdx, val.Tick, val.Timestamp,
			literal(at.Format("2006-01-02 15:04:05.999999")),
			literal(at.Format("2006-01-02 15:04:05.999999")),
			literal(val.Scope), literal(val.Origin),
			metricsMap, metaMap,
		)
		batchRows = append(batchRows, rowStr)

		if len(batchRows) >= batchSize {
			if err := flush(); err != nil {
				t.Fatalf("flush failed at row %d: %v", imported, err)
			}
		}

		if imported%100 == 0 {
			t.Logf("Downloaded %d measurements (elapsed: %v)", imported, time.Since(start))
		}
		if imported >= 500 {
			break
		}
	}

	if err := flush(); err != nil {
		t.Fatalf("final flush failed: %v", err)
	}

	t.Logf("Imported %d measurements from S3 into DuckDB in %v", imported, time.Since(start))

	var total int64
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM symmwarehouse.hindsight.measurements").Scan(&total)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	t.Logf("Total rows in DuckDB: %d", total)
}
