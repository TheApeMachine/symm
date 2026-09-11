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
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/marcboeker/go-duckdb/v2"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

/*
Warehouse is a DuckDB engine with the Iceberg warehouse attached.

The engine connects on first use rather than at boot. It is reached only from
the dashboard, while the process that hosts it trades; a catalog that is down,
or an extension repository that cannot be reached, must cost an analyst an
error message rather than cost the run its start.
*/
type Warehouse struct {
	// mu guards the lazy construction of db and conn, and is held only for
	// that. running serializes the statements themselves, which share one
	// connection and therefore one session.
	mu      sync.Mutex
	running sync.Mutex
	db      *sql.DB
	conn    *sql.Conn
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
	viper.SetDefault("workbench.max_temp_directory_size", "10GB")
	viper.SetDefault("workbench.threads", 4)

	return &Warehouse{}
}

/*
Wrap adapts an already-open DuckDB. Production connects through connect; tests
attach their own tables to an in-memory database, which exercises the same
projection, encoding, and listing without a catalog server.
*/
func Wrap(db *sql.DB) *Warehouse {
	return &Warehouse{db: db}
}

/*
Close releases the engine. A Warehouse that never connected owns nothing.
*/
func (warehouse *Warehouse) Close() error {
	warehouse.mu.Lock()
	defer warehouse.mu.Unlock()

	if warehouse.db == nil {
		return nil
	}

	db := warehouse.db
	conn := warehouse.conn
	warehouse.db = nil
	warehouse.conn = nil

	if conn != nil {
		if err := conn.Close(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"workbench: close duckdb session",
				err,
			))
		}
	}

	if err := db.Close(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"workbench: close duckdb",
			err,
		))
	}

	return nil
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
	warehouse.mu.Lock()
	defer warehouse.mu.Unlock()

	if warehouse.db != nil {
		return warehouse.db, nil
	}

	uri := viper.GetString("storage.iceberg.uri")
	location := viper.GetString("storage.iceberg.warehouse")

	if uri == "" || location == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"workbench: storage.iceberg.uri and storage.iceberg.warehouse are both required",
			nil,
		))
	}

	db, err := sql.Open("duckdb", "")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"workbench: open duckdb",
			err,
		))
	}

	catalog := catalogName(location)

	for _, statement := range warehouse.boot(uri, location, catalog) {
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

	warehouse.db = db

	return db, nil
}

/*
boot is the statement sequence that turns a blank DuckDB into a reader of this
deployment's Iceberg warehouse.

The arrow extension is a community extension: it supplies to_arrow_ipc, which
is how a result leaves this package. The iceberg extension resolves the REST
catalog. Both are cached on disk after their first install.

SeaweedFS serves both the catalog and the object store unauthenticated, so the
S3 secret carries placeholder credentials for the same reason hindsight/tables
does — an SDK given no credentials at all walks its provider chain out to the
EC2 metadata service — and the catalog is attached with authorization declared
absent rather than left to default to OAuth2.
*/
func (warehouse *Warehouse) boot(uri, location, catalog string) []string {
	endpoint := viper.GetString("storage.s3.endpoint")
	secure := strings.HasPrefix(endpoint, "https://")

	return []string{
		/*
			The engine shares a process with the trading path, and DuckDB's
			default ceiling is most of the machine's memory. An unpivoted view
			of a table is materialized in full before it is paged, and the tape
			tables run to tens of millions of rows carrying a payload each, so
			an analyst opening one would otherwise take the run's memory with
			it. The ceiling is declared instead, and overflowing it is refused
			rather than spilled: a query that will not fit is the analyst's to
			narrow, and a run that is holding positions is not the thing that
			should pay for it.
		*/
		fmt.Sprintf("SET GLOBAL memory_limit=%s", literal(
			viper.GetString("workbench.memory_limit"),
		)),
		fmt.Sprintf("SET GLOBAL max_temp_directory_size=%s", literal(
			viper.GetString("workbench.max_temp_directory_size"),
		)),
		fmt.Sprintf("SET GLOBAL threads=%d", viper.GetInt("workbench.threads")),
		"SET GLOBAL preserve_insertion_order=false",
		// The viewer orders an unpivoted view by a row identity, and an
		// Iceberg table has none to offer: its rows have no order the catalog
		// guarantees, so the handler asks for `ORDER BY NULL` instead. DuckDB
		// refuses a constant sort key unless told that a no-op ordering is
		// meant, and having been told, drops the sort from the plan entirely
		// rather than sorting millions of rows on one repeated value.
		"SET GLOBAL order_by_non_integer_literal=true",
		"INSTALL arrow FROM community",
		"LOAD arrow",
		"INSTALL iceberg",
		"LOAD iceberg",
		fmt.Sprintf(
			"CREATE OR REPLACE SECRET warehouse_store ("+
				"TYPE s3, KEY_ID %s, SECRET %s, REGION %s, "+
				"ENDPOINT %s, URL_STYLE 'path', USE_SSL %t)",
			literal(accessKeyID()),
			literal(secretAccessKey()),
			literal(viper.GetString("storage.s3.region")),
			literal(strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")),
			secure,
		),
		fmt.Sprintf(
			"ATTACH %s AS %s (TYPE iceberg, ENDPOINT %s, AUTHORIZATION_TYPE 'none')",
			literal(location), identifier(catalog), literal(uri),
		),
	}
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
	db, err := warehouse.connect(ctx)

	if err != nil {
		return nil, err
	}

	warehouse.mu.Lock()
	defer warehouse.mu.Unlock()

	if warehouse.conn != nil {
		return warehouse.conn, nil
	}

	conn, err := db.Conn(ctx)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"workbench: open duckdb session",
			err,
		))
	}

	warehouse.conn = conn

	return conn, nil
}

var selectWithoutSelection = regexp.MustCompile(`(?i)\bSELECT\s+FROM\b`)

/*
sanitizeStatement repairs common client template anomalies such as a SELECT clause
emitted without a selection list (e.g. `SELECT FROM ...`), replacing with `SELECT NULL FROM ...`.
*/
func sanitizeStatement(statement string) string {
	return selectWithoutSelection.ReplaceAllString(statement, "SELECT NULL FROM")
}

/*
Execute runs one statement and returns its result as an Apache Arrow IPC
stream. A statement that produces no rows — the viewer materializing or
dropping a view — returns an empty stream rather than an error.

to_arrow_ipc emits the stream in pieces: a schema message followed by one
message per record batch, which concatenate into the stream the viewer reads.
*/
func (warehouse *Warehouse) Execute(ctx context.Context, statement string) ([]byte, error) {
	statement = sanitizeStatement(statement)

	conn, err := warehouse.session(ctx)

	if err != nil {
		return nil, err
	}

	warehouse.running.Lock()
	defer warehouse.running.Unlock()

	rows, err := warehouse.selects(ctx, conn, statement)

	if err != nil {
		return nil, err
	}

	if !rows {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"workbench: execute statement: "+statement,
				err,
			))
		}

		return []byte{}, nil
	}

	projected, err := warehouse.project(ctx, conn, statement)

	if err != nil {
		return nil, err
	}

	return warehouse.stream(ctx, conn, projected)
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

	stream := make([]byte, 0)

	for rows.Next() {
		var message []byte

		if err := rows.Scan(&message); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"workbench: scan arrow ipc message",
				err,
			))
		}

		stream = append(stream, message...)
	}

	if err := rows.Err(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"workbench: read arrow ipc stream",
			err,
		))
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
