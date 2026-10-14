/*
Package workbench is the analytical query engine behind the dashboard's
Analytical Workbench surface.

Hindsight's record families are Iceberg tables in a SeaweedFS table bucket, and
hindsight/tables reads them one record family at a time through typed scans
because the live system needs typed rows. Analysis needs the opposite: an
arbitrary SQL expression over any combination of those tables, evaluated where
the data is rather than in a browser.

DuckDB attaches the same Iceberg catalog and answers that SQL server-side. The
answer leaves as an Apache Arrow IPC stream, which is the format the
Perspective viewer loads directly, so no row-by-row JSON encoding stands
between the scan and the screen.

The statements are not written by a person. The dashboard runs Perspective as a
Virtual Server: the viewer translates its own configuration — group by, filter,
sort, expressions — into SQL and asks for only the window it is displaying, so
the engine sees a stream of small statements, some of which materialize a view
and return no rows at all.

The engine is deliberately separate from hindsight/tables: that package is
imported by the live capture and decision paths, and DuckDB is a large cgo
dependency that has no business in them.
*/
package workbench

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/marcboeker/go-duckdb/v2"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/store"
)

/*
Warehouse is a DuckDB engine with the Iceberg warehouse attached.

The engine connects on first use rather than at boot. It is reached only from
the dashboard, while the process that hosts it trades; a catalog that is down,
or an extension repository that cannot be reached, must cost an analyst an
error message rather than cost the run its start.
*/
type Warehouse struct {
	db          atomic.Pointer[sql.DB]
	conn        atomic.Pointer[sql.Conn]
	sessionGate chan struct{}
}

/*
Column is one column of a statement's result.
*/
type Column struct {
	Name string
	Type string
}

/*
New constructs the engine without connecting.
*/
func New() *Warehouse {
	viper.SetDefault("workbench.memory_limit", "4GB")
	viper.SetDefault("workbench.max_temp_directory_size", "64GB")
	viper.SetDefault("workbench.threads", 4)

	warehouse := &Warehouse{
		sessionGate: make(chan struct{}, 1),
	}
	warehouse.sessionGate <- struct{}{}

	return warehouse
}

/*
Close releases the engine. A Warehouse that never connected owns nothing.
*/
func (warehouse *Warehouse) Close() error {
	conn := warehouse.conn.Swap(nil)
	db := warehouse.db.Swap(nil)

	if conn != nil {
		if err := conn.Close(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"workbench: close duckdb session",
				err,
			))
		}
	}

	if db != nil {
		if err := db.Close(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"workbench: close duckdb",
				err,
			))
		}
	}

	return nil
}

/*
DB returns the underlying database handle with the Iceberg warehouse attached.
*/
func (warehouse *Warehouse) DB(ctx context.Context) (*sql.DB, error) {
	return warehouse.connect(ctx)
}

/*
connect opens DuckDB and attaches the Iceberg warehouse, once.

Extensions, secrets, and attachments belong to the database instance rather
than to the connection that declared them, so this runs on one connection and
every pooled connection afterwards sees the catalog.

A failed connection leaves the engine unconnected rather than latching the
error: the catalog it could not reach is a separate process, and the next
request is a legitimate retry.
*/
func (warehouse *Warehouse) connect(ctx context.Context) (*sql.DB, error) {
	if existing := warehouse.db.Load(); existing != nil {
		return existing, nil
	}

	db, err := sql.Open("duckdb", "")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"workbench: open duckdb",
			err,
		))
	}

	for _, statement := range warehouse.boot() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			if closeErr := db.Close(); closeErr != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.IO,
					"workbench: close duckdb after failed boot",
					closeErr,
				))
			}

			return nil, errnie.Error(errnie.Err(
				errnie.BadGateway,
				"workbench: "+statement,
				err,
			))
		}
	}

	if !warehouse.db.CompareAndSwap(nil, db) {
		if closeErr := db.Close(); closeErr != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"workbench: close redundant duckdb instance",
				closeErr,
			))
		}

		return warehouse.db.Load(), nil
	}

	return db, nil
}

/*
boot is the statement sequence that initializes DuckDB with the analytical
measurements table, views, Arrow extension, and S3 credentials.
*/
func (warehouse *Warehouse) boot() []string {
	endpoint := viper.GetString("storage.s3.endpoint")
	secure := strings.HasPrefix(endpoint, "https://")

	statements := []string{
		fmt.Sprintf("SET GLOBAL memory_limit=%s", literal(
			viper.GetString("workbench.memory_limit"),
		)),
		fmt.Sprintf("SET GLOBAL max_temp_directory_size=%s", literal(
			viper.GetString("workbench.max_temp_directory_size"),
		)),
		fmt.Sprintf("SET GLOBAL threads=%d", viper.GetInt("workbench.threads")),
		"SET GLOBAL preserve_insertion_order=true",
		"SET GLOBAL enable_http_metadata_cache=true",
		"SET GLOBAL parquet_metadata_cache=true",
		"SET GLOBAL prefetch_all_parquet_files=true",
		"SET GLOBAL order_by_non_integer_literal=true",
		"INSTALL arrow FROM community",
		"LOAD arrow",
	}

	if endpoint != "" {
		statements = append(statements, fmt.Sprintf(
			"CREATE OR REPLACE SECRET warehouse_store ("+
				"TYPE s3, KEY_ID %s, SECRET %s, REGION %s, "+
				"ENDPOINT %s, URL_STYLE 'path', USE_SSL %t)",
			literal(accessKeyID()),
			literal(secretAccessKey()),
			literal(viper.GetString("storage.s3.region")),
			literal(strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")),
			secure,
		))
	}

	statements = append(statements,
		"ATTACH ':memory:' AS symmwarehouse",
		"CREATE SCHEMA IF NOT EXISTS symmwarehouse.hindsight",
		`CREATE TABLE IF NOT EXISTS symmwarehouse.hindsight.measurements (
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
		)`,
		"CREATE OR REPLACE VIEW symmwarehouse.hindsight.spot_trade AS SELECT * FROM symmwarehouse.hindsight.measurements WHERE source = 'spot:trade'",
		"CREATE OR REPLACE VIEW symmwarehouse.hindsight.spot_ticker AS SELECT * FROM symmwarehouse.hindsight.measurements WHERE source = 'spot:ticker'",
		"CREATE OR REPLACE VIEW symmwarehouse.hindsight.runs AS SELECT epoch, min(\"at\") AS started_at, max(\"at\") AS ended_at, count(*) AS measurement_count FROM symmwarehouse.hindsight.measurements GROUP BY epoch",
	)

	return statements
}

/*
Ingest streams measurements from the store client and batch-inserts them into
the symmwarehouse.hindsight.measurements table. It returns the number of ingested measurements.
*/
func (warehouse *Warehouse) Ingest(ctx context.Context, client *store.Client, prefix string) (int, error) {
	if client == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "[warehouse] store client is required for ingest", nil,
		))
	}

	db, err := warehouse.DB(ctx)

	if err != nil {
		return 0, err
	}

	batchSize := 200
	var batchRows []string
	imported := 0

	flush := func() error {
		if len(batchRows) == 0 {
			return nil
		}

		query := "INSERT INTO symmwarehouse.hindsight.measurements VALUES " + strings.Join(batchRows, ",")
		_, execErr := db.ExecContext(ctx, query)
		batchRows = batchRows[:0]

		if execErr != nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "[warehouse] batch insert failed", execErr,
			))
		}

		return nil
	}

	for measurement := range client.Get(prefix) {
		select {
		case <-ctx.Done():
			return imported, errnie.Error(ctx.Err())
		default:
		}

		if measurement == nil {
			continue
		}

		imported++

		var metricKeys []string
		var metricVals []string

		for entry := range measurement.Read() {
			if entry == nil || entry.Metric == nil {
				continue
			}

			metricKeys = append(metricKeys, literal(entry.Key))
			exactStr := "''"

			if entry.Metric.Exact() != nil {
				exactStr = literal(entry.Metric.Exact().String())
			}

			metricVals = append(metricVals, fmt.Sprintf(
				"{'raw': %f, 'snm': %f, 'exact': %s, 'unit': %s, 'timescale': %s}",
				entry.Metric.Raw(), entry.Metric.SNM(), exactStr,
				literal(string(entry.Metric.Unit())),
				literal(string(entry.Metric.Timescale())),
			))
		}

		metricsMap := fmt.Sprintf("map([%s], [%s])", strings.Join(metricKeys, ", "), strings.Join(metricVals, ", "))

		var metaKeys []string
		var metaVals []string

		for entry := range measurement.Meta() {
			if entry != nil {
				metaKeys = append(metaKeys, literal(entry.Key))
				metaVals = append(metaVals, literal(entry.Value))
			}
		}

		metaMap := fmt.Sprintf("map([%s], [%s])", strings.Join(metaKeys, ", "), strings.Join(metaVals, ", "))

		rowStr := fmt.Sprintf(
			"(%d, %d, %d, %d, %d, %s, %s, %s, %s, %s, %s)",
			measurement.ID, measurement.Epoch, measurement.SeqIdx, measurement.Tick, measurement.Timestamp,
			literal(measurement.At.Format("2006-01-02 15:04:05.999999")),
			literal(measurement.From.Format("2006-01-02 15:04:05.999999")),
			literal(measurement.Label), literal(measurement.Source),
			metricsMap, metaMap,
		)
		batchRows = append(batchRows, rowStr)

		if len(batchRows) >= batchSize {
			if err := flush(); err != nil {
				return imported, err
			}
		}
	}

	if err := flush(); err != nil {
		return imported, err
	}

	return imported, nil
}

// accessKeyID is the S3 key the FileIO signs with, placeheld when anonymous.
func accessKeyID() string {
	if viper.GetBool("storage.s3.anonymous") {
		return "anonymous"
	}

	return viper.GetString("storage.s3.access_key_id")
}

// secretAccessKey is the S3 secret the FileIO signs with, placeheld when anonymous.
func secretAccessKey() string {
	if viper.GetBool("storage.s3.anonymous") {
		return "anonymous"
	}

	return viper.GetString("storage.s3.secret_access_key")
}

/*
catalogName derives the DuckDB catalog alias from the warehouse location, so
SQL addresses the table bucket by the name it has in the object store rather
than by an alias only this package knows.
*/
func catalogName(location string) string {
	trimmed := strings.Trim(strings.TrimPrefix(location, "s3://"), "/")

	if bucket, _, found := strings.Cut(trimmed, "/"); found {
		return bucket
	}

	return trimmed
}

/*
session returns the engine's one pinned connection.

The viewer's statements are a session, not a set of independent queries: it
materializes a view under a name and then reads, measures, and finally drops
that name across later statements. Holding one connection keeps every statement
in the same session, and serializing on it keeps two viewers from racing to
create the same name.
*/
func (warehouse *Warehouse) session(ctx context.Context) (*sql.Conn, error) {
	if existing := warehouse.conn.Load(); existing != nil {
		return existing, nil
	}

	db, err := warehouse.connect(ctx)

	if err != nil {
		return nil, err
	}

	conn, err := db.Conn(ctx)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"workbench: open duckdb session",
			err,
		))
	}

	if !warehouse.conn.CompareAndSwap(nil, conn) {
		if closeErr := conn.Close(); closeErr != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"workbench: close redundant duckdb session",
				closeErr,
			))
		}

		return warehouse.conn.Load(), nil
	}

	return conn, nil
}

var (
	selectWithoutSelection = regexp.MustCompile(`(?i)\bSELECT\s+FROM\b`)
	createTableAsPattern   = regexp.MustCompile(`(?i)\bCREATE\s+(?:OR\s+REPLACE\s+)?TABLE\s+([^\s]+)\s+AS\b`)
	dropTablePattern       = regexp.MustCompile(`(?i)\bDROP\s+TABLE\b`)
)

/*
sanitizeStatement repairs common client template anomalies such as a SELECT clause
emitted without a selection list (e.g. `SELECT FROM ...`), replacing with `SELECT NULL FROM ...`.
It also converts large temporary table materializations into virtual views (`CREATE OR REPLACE VIEW ... AS`)
so analytical queries execute with zero disk footprint and Parquet filter pushdown.
*/
func sanitizeStatement(statement string) string {
	statement = selectWithoutSelection.ReplaceAllString(statement, "SELECT NULL FROM")
	statement = createTableAsPattern.ReplaceAllString(statement, "CREATE OR REPLACE VIEW $1 AS")
	return dropTablePattern.ReplaceAllString(statement, "DROP VIEW")
}

/*
Execute runs one statement and returns its result as an Apache Arrow IPC
stream. A statement that produces no rows — the viewer materializing or
dropping a view — returns an empty stream rather than an error.

to_arrow_ipc emits the stream in pieces: a schema message followed by one
message per record batch, which concatenate into the stream the viewer reads.
*/
func (warehouse *Warehouse) Execute(ctx context.Context, statement string) ([]byte, error) {
	statement = strings.TrimSpace(sanitizeStatement(statement))
	if statement == "" {
		return []byte{}, nil
	}

	conn, err := warehouse.session(ctx)

	if err != nil {
		return nil, err
	}

	select {
	case <-warehouse.sessionGate:
		defer func() { warehouse.sessionGate <- struct{}{} }()
	case <-ctx.Done():
		return nil, errnie.Error(ctx.Err())
	}

	rows, err := warehouse.selects(ctx, conn, statement)

	if err != nil {
		return nil, err
	}

	if rows {
		projected, err := warehouse.project(ctx, conn, statement)

		if err != nil {
			return nil, err
		}

		return warehouse.stream(ctx, conn, projected)
	}

	_, execErr := conn.ExecContext(ctx, statement)

	if execErr == nil {
		return []byte{}, nil
	}

	if strings.Contains(execErr.Error(), "is of type View") {
		dropViewStatement := dropTablePattern.ReplaceAllString(statement, "DROP VIEW")

		if _, dropViewErr := conn.ExecContext(ctx, dropViewStatement); dropViewErr == nil {
			return []byte{}, nil
		}
	}

	return nil, errnie.Error(errnie.Err(
		errnie.Validation,
		"workbench: execute statement: "+statement,
		execErr,
	))
}

/*
selects reports whether a statement produces a result set, by asking DuckDB's
own parser what kind of statement it is rather than reading the SQL. Every form
the viewer asks data of — SELECT, SHOW, DESCRIBE — parses as a select; the
forms that materialize or drop a view do not.
*/
func (warehouse *Warehouse) selects(
	ctx context.Context, conn *sql.Conn, statement string,
) (bool, error) {
	var kind duckdb.StmtType

	err := conn.Raw(func(driverConn any) error {
		prepared, err := driverConn.(*duckdb.Conn).PrepareContext(ctx, statement)

		if err != nil {
			return err
		}

		defer prepared.Close()

		kind, err = prepared.(*duckdb.Stmt).StatementType()

		return err
	})

	if err != nil {
		return false, errnie.Error(errnie.Err(
			errnie.Validation,
			"workbench: prepare statement: "+statement,
			err,
		))
	}

	return kind == duckdb.STATEMENT_TYPE_SELECT, nil
}

/*
stream evaluates a select and concatenates the Arrow IPC messages it produces.
*/
func (warehouse *Warehouse) stream(
	ctx context.Context, conn *sql.Conn, statement string,
) ([]byte, error) {
	rows, err := conn.QueryContext(ctx, "SELECT ipc FROM to_arrow_ipc(("+statement+"))")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"workbench: evaluate query",
			err,
		))
	}

	defer rows.Close()

	var schemaMessages [][]byte
	var dictMessages [][]byte
	var recordMessages [][]byte
	var otherMessages [][]byte

	for rows.Next() {
		var message []byte

		if err := rows.Scan(&message); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"workbench: scan arrow ipc message",
				err,
			))
		}

		messageReader := ipc.NewMessageReader(bytes.NewReader(message))
		parsedMessage, parseErr := messageReader.Message()

		if parseErr != nil {
			otherMessages = append(otherMessages, message)
			continue
		}

		switch parsedMessage.Type() {
		case ipc.MessageSchema:
			schemaMessages = append(schemaMessages, message)
		case ipc.MessageDictionaryBatch:
			dictMessages = append(dictMessages, message)
		case ipc.MessageRecordBatch:
			recordMessages = append(recordMessages, message)
		default:
			otherMessages = append(otherMessages, message)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"workbench: read arrow ipc stream",
			err,
		))
	}

	totalLength := 0

	for _, chunk := range schemaMessages {
		totalLength += len(chunk)
	}

	for _, chunk := range dictMessages {
		totalLength += len(chunk)
	}

	for _, chunk := range recordMessages {
		totalLength += len(chunk)
	}

	for _, chunk := range otherMessages {
		totalLength += len(chunk)
	}

	stream := make([]byte, 0, totalLength)

	for _, chunk := range schemaMessages {
		stream = append(stream, chunk...)
	}

	for _, chunk := range dictMessages {
		stream = append(stream, chunk...)
	}

	for _, chunk := range recordMessages {
		stream = append(stream, chunk...)
	}

	for _, chunk := range otherMessages {
		stream = append(stream, chunk...)
	}

	return stream, nil
}

/*
project rewrites a query's nested columns to JSON text.

The viewer's table is flat: it refuses a MAP column outright, and it flattens a
LIST column into one row per element, which silently multiplies the row count
of the answer. Neither is recoverable in the browser, so a column DuckDB
reports as nested leaves as its JSON rendering — visible as text, and counted
as one row.

Everything else passes through untouched, decimals included: Arrow carries them
as decimals and the viewer reads them as numbers.
*/
func (warehouse *Warehouse) project(
	ctx context.Context, conn *sql.Conn, statement string,
) (string, error) {
	columns, err := warehouse.describe(ctx, conn, statement)

	if err != nil {
		return "", err
	}

	if len(columns) == 0 {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"workbench: query selects no columns",
			nil,
		))
	}

	selections := make([]string, 0, len(columns))

	for _, column := range columns {
		selections = append(selections, selection(column.Name, column.Type))
	}

	return "SELECT " + strings.Join(selections, ", ") + " FROM (" + statement + ")", nil
}

/*
describe reports the columns a statement produces, without evaluating it.
*/
func (warehouse *Warehouse) describe(
	ctx context.Context, conn *sql.Conn, statement string,
) ([]Column, error) {
	rows, err := conn.QueryContext(ctx, "DESCRIBE ("+statement+")")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"workbench: describe query",
			err,
		))
	}

	defer rows.Close()

	columns := make([]Column, 0)

	for rows.Next() {
		var name, kind string
		var null, key, def, extra sql.NullString

		if err := rows.Scan(&name, &kind, &null, &key, &def, &extra); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"workbench: scan query description",
				err,
			))
		}

		columns = append(columns, Column{Name: name, Type: kind})
	}

	if err := rows.Err(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"workbench: read query description",
			err,
		))
	}

	return columns, nil
}

/*
selection is one column of the projection: the column itself, or its JSON
rendering when its type is nested.
*/
func selection(name, kind string) string {
	quoted := identifier(name)

	if !nested(kind) {
		return quoted
	}

	return "to_json(" + quoted + ")::VARCHAR AS " + quoted
}

/*
nested reports whether a DuckDB type is one the viewer cannot hold in a cell.
DuckDB renders these types structurally, so the composite is recognizable from
its rendering: a list by its suffix, the keyed types by their prefix.
*/
func nested(kind string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(kind))

	if strings.HasSuffix(normalized, "[]") || strings.HasSuffix(normalized, ")[]") {
		return true
	}

	for _, prefix := range []string{"MAP(", "STRUCT(", "UNION(", "LIST("} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}

	return false
}

// literal renders a SQL string literal.
func literal(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// identifier renders a quoted SQL identifier.
func identifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
