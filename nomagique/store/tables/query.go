package tables

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/marcboeker/go-duckdb/v2"
	"github.com/theapemachine/errnie"
)

/*
QueryServer owns the Cap’n Proto analytical session. Configuration is idle;
SQL and catalog I/O run on demand through authored writes or Query RPCs.
*/
type QueryServer struct {
	// running owns the database and pinned session throughout each operation.
	configuration sync.RWMutex
	running       sync.Mutex
	db            *sql.DB
	conn          *sql.Conn
	setup         []string
	configured    bool
	statement     string
	arguments     []any
	operation     *queryOperation
	superseded    uint64
	cancel        context.CancelFunc
	ctx           context.Context
}

/* queryOperation owns one read and its immutable parameters until Done consumes it. */
type queryOperation struct {
	statement string
	arguments []any
	finished  chan struct{}
	output    []byte
	err       error
}

/*
queryColumn is one column of a statement's result.
*/
type queryColumn struct {
	Name string
	Type string
}

/*
NewQuery constructs the engine without connecting.
*/
func NewQuery() *QueryServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &QueryServer{ctx: ctx, cancel: cancel}
}

/*
Close releases the engine. A QueryServer that never connected owns nothing.
*/
func (warehouse *QueryServer) Close() error {
	warehouse.cancel()
	warehouse.configuration.RLock()
	operation := warehouse.operation
	warehouse.configuration.RUnlock()
	if operation != nil {
		<-operation.finished
	}
	warehouse.running.Lock()
	defer warehouse.running.Unlock()

	if warehouse.db == nil {
		return nil
	}

	db := warehouse.db
	conn := warehouse.conn
	warehouse.db = nil
	warehouse.conn = nil

	var err error

	if conn != nil {
		err = conn.Close()
	}

	if err := errors.Join(err, db.Close()); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "query: close session", err))
	}

	return nil
}

/*
connect lazily opens the graph-configured analytical database. A failed
connection remains unconnected, permitting the next request to retry.
*/
func (warehouse *QueryServer) connect(ctx context.Context) (*sql.DB, error) {

	if warehouse.db != nil {
		return warehouse.db, nil
	}

	warehouse.configuration.RLock()
	configured := warehouse.configured
	setup := warehouse.setup
	warehouse.configuration.RUnlock()

	if !configured {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "query: setup has not been configured", nil))
	}

	db, err := sql.Open("duckdb", "")

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.IO, "workbench: open duckdb", err))
	}

	for _, statement := range setup {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "query: configure analytical session", errors.Join(err, db.Close())))
		}
	}

	warehouse.db = db

	return db, nil
}

/*
session pins one connection so temporary views survive between requests.
*/
func (warehouse *QueryServer) session(ctx context.Context) (*sql.Conn, error) {
	db, err := warehouse.connect(ctx)

	if err != nil {
		return nil, err
	}

	if warehouse.conn != nil {
		return warehouse.conn, nil
	}

	conn, err := db.Conn(ctx)

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.IO, "workbench: open duckdb session", err))
	}

	warehouse.conn = conn

	return conn, nil
}

/*
execute returns Arrow IPC for row-producing statements and an empty body for DDL.
*/
func (warehouse *QueryServer) execute(ctx context.Context, statement string) ([]byte, error) {
	warehouse.running.Lock()
	defer warehouse.running.Unlock()
	conn, err := warehouse.session(ctx)

	if err != nil {
		return nil, err
	}

	rows, err := warehouse.selects(ctx, conn, statement)

	if err != nil {
		return nil, err
	}

	if !rows {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "workbench: execute statement: "+statement, err))
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
selects asks DuckDB’s parser whether a statement produces rows.
*/
func (warehouse *QueryServer) selects(
	ctx context.Context, conn *sql.Conn, statement string,
) (bool, error) {
	var kind duckdb.StmtType

	err := conn.Raw(func(driverConn any) (err error) {
		prepared, err := driverConn.(*duckdb.Conn).PrepareContext(ctx, statement)

		if err != nil {
			return err
		}

		defer func() { err = errors.Join(err, prepared.Close()) }()

		kind, err = prepared.(*duckdb.Stmt).StatementType()

		return err
	})

	if err != nil {
		return false, errnie.Error(errnie.Err(errnie.Validation, "workbench: prepare statement: "+statement, err))
	}

	return kind == duckdb.STATEMENT_TYPE_SELECT, nil
}

/*
stream evaluates a select and concatenates the Arrow IPC messages it produces.
*/
func (warehouse *QueryServer) stream(
	ctx context.Context, conn *sql.Conn, statement string,
) (output []byte, err error) {
	rows, err := conn.QueryContext(ctx, "SELECT ipc FROM to_arrow_ipc(("+statement+"))")

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "workbench: evaluate query", err))
	}

	defer func() { err = errors.Join(err, rows.Close()) }()

	stream := make([]byte, 0)

	for rows.Next() {
		var message []byte

		if err := rows.Scan(&message); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.IO, "workbench: scan arrow ipc message", err))
		}

		stream = append(stream, message...)
	}

	if err := rows.Err(); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.IO, "workbench: read arrow ipc stream", err))
	}

	return stream, nil
}

/*
project preserves nested columns as JSON text, which Perspective can display
without rejecting maps or multiplying rows for list elements.
*/
func (warehouse *QueryServer) project(
	ctx context.Context, conn *sql.Conn, statement string,
) (string, error) {
	columns, err := warehouse.describe(ctx, conn, statement)

	if err != nil {
		return "", err
	}

	if len(columns) == 0 {
		return "", errnie.Error(errnie.Err(errnie.Validation, "workbench: query selects no columns", nil))
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
func (warehouse *QueryServer) describe(
	ctx context.Context, conn *sql.Conn, statement string,
) (output []queryColumn, err error) {
	rows, err := conn.QueryContext(ctx, "DESCRIBE ("+statement+")")

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "workbench: describe query", err))
	}

	defer func() { err = errors.Join(err, rows.Close()) }()

	columns := make([]queryColumn, 0)

	for rows.Next() {
		var name, kind string
		var null, key, def, extra sql.NullString

		if err := rows.Scan(&name, &kind, &null, &key, &def, &extra); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.IO, "workbench: scan query description", err))
		}

		columns = append(columns, queryColumn{Name: name, Type: kind})
	}

	if err := rows.Err(); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.IO, "workbench: read query description", err))
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
nested recognizes DuckDB’s composite type syntax.
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

// identifier renders a quoted SQL identifier.
func identifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

/* Write configures the analytical session without opening it on the market path. */
func (warehouse *QueryServer) Write(ctx context.Context, call Query_write) error {
	input, err := call.Args().Setup()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "query: setup", err))
	}
	setup := make([]string, input.Len())

	for index := range input.Len() {
		setup[index], err = input.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "query: setup statement", err))
		}
	}
	warehouse.configuration.Lock()
	defer warehouse.configuration.Unlock()

	if warehouse.configured && !slices.Equal(setup, warehouse.setup) {
		return errnie.Error(errnie.Err(errnie.Validation, "query: session setup cannot change", nil))
	}
	warehouse.setup, warehouse.configured = setup, true
	statement, err := call.Args().Sql()
	if err != nil {
		return errnie.Error(err)
	}
	parameters, err := call.Args().Parameters()
	if err != nil {
		return errnie.Error(err)
	}
	warehouse.arguments = make([]any, parameters.Len())
	for index := range parameters.Len() {
		warehouse.arguments[index], err = parameters.At(index)
		if err != nil {
			return errnie.Error(err)
		}
	}
	warehouse.statement = strings.TrimSuffix(strings.TrimSpace(statement), ";")
	warehouse.dispatch()
	return nil
}

/* dispatch starts one authored read while configuration is locked. */
func (warehouse *QueryServer) dispatch() {
	if warehouse.operation != nil || warehouse.statement == "" {
		return
	}
	operation := &queryOperation{statement: warehouse.statement, arguments: slices.Clone(warehouse.arguments), finished: make(chan struct{})}
	warehouse.operation = operation
	go func() {
		operation.output, operation.err = warehouse.jsonRows(warehouse.ctx, operation.statement, operation.arguments)
		close(operation.finished)
	}()
}

/* Done reports readiness and publishes a completed read once without waiting on SQL. */
func (warehouse *QueryServer) Done(ctx context.Context, call Query_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	warehouse.configuration.Lock()
	defer warehouse.configuration.Unlock()
	result.SetConfigured(warehouse.configured)
	result.SetSuperseded(warehouse.superseded)
	operation := warehouse.operation
	if operation == nil {
		return nil
	}
	select {
	case <-operation.finished:
	default:
		result.SetPending(true)
		return nil
	}
	warehouse.operation = nil
	if operation.err != nil {
		return operation.err
	}
	if operation.statement != warehouse.statement || !slices.Equal(operation.arguments, warehouse.arguments) {
		warehouse.superseded++
		result.SetSuperseded(warehouse.superseded)
		warehouse.dispatch()
		result.SetPending(warehouse.operation != nil)
		return nil
	}
	warehouse.statement, warehouse.arguments = "", nil
	return errnie.Error(result.SetOut(operation.output))
}

/* Query executes a statement through this node's one database session. */
func (warehouse *QueryServer) Query(ctx context.Context, call Query_query) error {
	// Analytical work yields node dispatch, so graph configuration and Done never wait for SQL.
	call.Go()
	statement, err := call.Args().Sql()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "query: statement", err))
	}

	statement = strings.TrimSuffix(strings.TrimSpace(statement), ";")

	if strings.TrimSpace(statement) == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "query: statement is empty", nil))
	}
	parameters, err := call.Args().Parameters()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "query: parameters", err))
	}
	arguments := make([]any, parameters.Len())
	for index := range parameters.Len() {
		arguments[index], err = parameters.At(index)
		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "query: parameter", err))
		}
	}
	var output []byte
	if call.Args().Json() {
		output, err = warehouse.jsonRows(ctx, statement, arguments)
	}
	if !call.Args().Json() {
		if len(arguments) != 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "query: Arrow SQL does not accept bound parameters", nil))
		}
		output, err = warehouse.execute(ctx, statement)
	}

	if err != nil {
		return err
	}
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "query: response", err))
	}

	if err := result.SetOut(output); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "query: response bytes", err))
	}
	return nil
}

/* jsonRows preserves DuckDB's typed JSON representation without Go row coercion. */
func (warehouse *QueryServer) jsonRows(ctx context.Context, statement string, arguments []any) (output []byte, err error) {
	warehouse.running.Lock()
	defer warehouse.running.Unlock()
	connection, err := warehouse.session(ctx)

	if err != nil {
		return nil, err
	}
	rows, err := connection.QueryContext(ctx, "SELECT to_json(record)::VARCHAR FROM ("+statement+") AS record", arguments...)

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.IO, "query: rows", err))
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	var buffer bytes.Buffer
	buffer.WriteByte('[')
	count := 0

	for rows.Next() {
		var record string

		if err := rows.Scan(&record); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.IO, "query: JSON row", err))
		}

		if count > 0 {
			buffer.WriteByte(',')
		}
		buffer.WriteString(record)
		count++
	}

	if err := rows.Err(); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.IO, "query: read rows", err))
	}
	buffer.WriteByte(']')
	return buffer.Bytes(), nil
}

/* Shutdown closes the engine with the lifetime of its Cap'n Proto capability. */
func (warehouse *QueryServer) Shutdown() {
	if err := warehouse.Close(); err != nil {
		errnie.Error(err)
	}
}
