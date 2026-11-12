package workbench

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
)

/*
Warehouse is a DuckDB engine with S3 JSON views for the Analytical Workbench.
*/
type Warehouse struct {
	catalog *Catalog
	mu      sync.Mutex
	running sync.Mutex
	db      *sql.DB
	conn    *sql.Conn
}

/*
New constructs the engine without connecting.
*/
func New() *Warehouse {
	return &Warehouse{
		catalog: NewCatalog(system.Cfg.Storage.S3.Bucket),
	}
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
				errnie.IO, "workbench: close duckdb session", err,
			))
		}
	}

	if err := db.Close(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "workbench: close duckdb", err,
		))
	}

	return nil
}

func (warehouse *Warehouse) connect(ctx context.Context) (*sql.DB, error) {
	warehouse.mu.Lock()
	defer warehouse.mu.Unlock()

	if warehouse.db != nil {
		return warehouse.db, nil
	}

	db, err := sql.Open("duckdb", "")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "workbench: open duckdb", err,
		))
	}

	for _, statement := range warehouse.boot() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			_ = db.Close()

			return nil, errnie.Error(errnie.Err(
				errnie.BadGateway, "workbench: "+statement, err,
			))
		}
	}

	for _, view := range warehouse.catalog.seeds() {
		if err := seedView(ctx, db, view); err != nil {
			_ = db.Close()

			return nil, err
		}
	}

	warehouse.db = db

	return db, nil
}

func seedView(ctx context.Context, db *sql.DB, view seed) error {
	if _, err := db.ExecContext(ctx, view.primary); err == nil {
		return nil
	} else if !emptyGlob(err) {
		return errnie.Error(errnie.Err(
			errnie.BadGateway, "workbench: "+view.primary, err,
		))
	}

	if _, err := db.ExecContext(ctx, view.empty); err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway, "workbench: "+view.empty, err,
		))
	}

	return nil
}

func emptyGlob(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()

	return strings.Contains(
		message, "No files found",
	) || strings.Contains(
		message, "No files found that match the pattern",
	)
}

func (warehouse *Warehouse) boot() []string {
	endpoint := system.Cfg.Storage.S3.Endpoint
	secure := strings.HasPrefix(endpoint, "https://")
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")

	keyID := system.Cfg.Storage.S3.AccessKeyID
	secret := system.Cfg.Storage.S3.SecretAccessKey

	if system.Cfg.Storage.S3.Anonymous {
		keyID = "anonymous"
		secret = "anonymous"
	}

	return []string{
		fmt.Sprintf("SET GLOBAL memory_limit=%s", literal(system.Cfg.Workbench.MemoryLimit)),
		fmt.Sprintf(
			"SET GLOBAL max_temp_directory_size=%s",
			literal(system.Cfg.Workbench.MaxTempDirectorySize),
		),
		fmt.Sprintf("SET GLOBAL threads=%d", system.Cfg.Workbench.Threads),
		"SET GLOBAL preserve_insertion_order=false",
		"SET GLOBAL order_by_non_integer_literal=true",
		"INSTALL httpfs",
		"LOAD httpfs",
		/*
			Open/closed lot JSON is rewritten in place while the desk runs.
			CREATE TABLE snapshots (and any long S3 scan) otherwise die with
			"ETag ... changed after it was opened". The workbench accepts a
			tear-consistent read of live prefixes rather than failing the viewer.
		*/
		"SET GLOBAL unsafe_disable_etag_checks=true",
		fmt.Sprintf(
			"CREATE OR REPLACE SECRET warehouse_store ("+
				"TYPE s3, KEY_ID %s, SECRET %s, REGION %s, "+
				"ENDPOINT %s, URL_STYLE 'path', USE_SSL %t)",
			literal(keyID),
			literal(secret),
			literal(system.Cfg.Storage.S3.Region),
			literal(host),
			secure,
		),
		// Named catalog keeps seeded views out of `memory`, which Perspective
		// reserves for its own materialized working tables.
		"ATTACH ':memory:' AS warehouse",
	}
}

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
			errnie.IO, "workbench: open duckdb session", err,
		))
	}

	warehouse.conn = conn

	return conn, nil
}

/*
Execute runs one statement and returns its result as an Apache Arrow IPC stream.
*/
func (warehouse *Warehouse) Execute(
	ctx context.Context, statement string,
) ([]byte, error) {
	statement = sanitizeStatement(statement)

	conn, err := warehouse.session(ctx)

	if err != nil {
		return nil, err
	}

	warehouse.running.Lock()
	defer warehouse.running.Unlock()

	selects, err := warehouse.selects(ctx, conn, statement)

	if err != nil {
		return nil, err
	}

	if !selects {
		if err := warehouse.exec(ctx, conn, statement); err != nil {
			return nil, err
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
exec runs a non-SELECT statement, retrying briefly when a live S3 object is
rewritten mid-scan. ETag checks are already disabled at boot; retries cover
hosts that still surface the race (or an older httpfs without the setting).
*/
func (warehouse *Warehouse) exec(
	ctx context.Context, conn *sql.Conn, statement string,
) error {
	var err error

	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"workbench: execute statement: "+statement,
					ctx.Err(),
				))
			case <-time.After(time.Duration(attempt) * 50 * time.Millisecond):
			}
		}

		_, err = conn.ExecContext(ctx, statement)

		if err == nil || !etagChanged(err) {
			break
		}
	}

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"workbench: execute statement: "+statement,
			err,
		))
	}

	return nil
}

func etagChanged(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()

	return strings.Contains(message, "ETag") &&
		strings.Contains(message, "changed after it was opened")
}

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

func (warehouse *Warehouse) stream(
	ctx context.Context, conn *sql.Conn, statement string,
) ([]byte, error) {
	columns, err := warehouse.describe(ctx, conn, statement)

	if err != nil {
		return nil, err
	}

	rows, err := conn.QueryContext(ctx, statement)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "workbench: evaluate query", err,
		))
	}

	defer rows.Close()

	values := make([][]any, 0)

	for rows.Next() {
		dest := make([]any, len(columns))
		ptrs := make([]any, len(columns))

		for i := range dest {
			ptrs[i] = &dest[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO, "workbench: scan query row", err,
			))
		}

		values = append(values, dest)
	}

	if err := rows.Err(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "workbench: read query rows", err,
		))
	}

	stream, err := encodeIPC(columns, values)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "workbench: encode arrow ipc", err,
		))
	}

	return stream, nil
}

func (warehouse *Warehouse) project(
	ctx context.Context, conn *sql.Conn, statement string,
) (string, error) {
	columns, err := warehouse.describe(ctx, conn, statement)

	if err != nil {
		return "", err
	}

	if len(columns) == 0 {
		return "", errnie.Error(errnie.Err(
			errnie.Validation, "workbench: query selects no columns", nil,
		))
	}

	selections := make([]string, 0, len(columns))

	for _, column := range columns {
		selections = append(selections, selection(column.Name, column.Type))
	}

	return "SELECT " + strings.Join(selections, ", ") + " FROM (" + statement + ")", nil
}

func (warehouse *Warehouse) describe(
	ctx context.Context, conn *sql.Conn, statement string,
) ([]Column, error) {
	rows, err := conn.QueryContext(ctx, "DESCRIBE ("+statement+")")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "workbench: describe query", err,
		))
	}

	defer rows.Close()

	columns := make([]Column, 0)

	for rows.Next() {
		var name, kind string
		var null, key, def, extra sql.NullString

		if err := rows.Scan(&name, &kind, &null, &key, &def, &extra); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO, "workbench: scan query description", err,
			))
		}

		columns = append(columns, Column{Name: name, Type: kind})
	}

	if err := rows.Err(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "workbench: read query description", err,
		))
	}

	return columns, nil
}
