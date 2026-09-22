package tables

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow/array"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

/*
IcebergTableServer appends what it is written to an Iceberg table.

Every append is a snapshot plus a metadata write, so rows are held until the
node is read rather than written one at a time. What is held is what has not
been acknowledged: a commit that fails leaves its rows here, and the next read
sends them again. Rows are never dropped to keep the node moving — a tape with
a hole in it cannot be replayed against.

The table is declared by config, a TableConfig as JSON. What the columns are
and what they are called is the caller's declaration, not this node's
knowledge.
*/
type IcebergTableServer struct {
	mutex   sync.Mutex
	catalog *Catalog
	opened  TableConfig
	table   *icetable.Table

	pending     [][]byte
	appendBytes int
	committed   int64
}

func NewIcebergTable() *IcebergTableServer {
	return &IcebergTableServer{
		appendBytes: DefaultStorageConfig().Iceberg.AppendBytes,
	}
}

/*
Write holds one payload for the next commit.
*/
func (server *IcebergTableServer) Write(ctx context.Context, call IcebergTable_write) error {
	config, err := call.Args().Config()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[iceberg] failed to read the table declaration",
			err,
		))
	}

	payload, err := call.Args().Payload()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[iceberg] failed to read the payload",
			err,
		))
	}

	if config == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] a table has to be declared before anything can be written to it",
			nil,
		))
	}

	if err := server.open(ctx, config); err != nil {
		return err
	}

	if len(payload) == 0 {
		return nil
	}

	server.mutex.Lock()
	server.pending = append(server.pending, bytes.Clone(payload))
	server.mutex.Unlock()

	return nil
}

/*
open resolves the declared table once, creating it when the catalog does not
hold it yet.
*/
func (server *IcebergTableServer) open(ctx context.Context, config string) error {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	if server.table != nil && server.opened.declaration == config {
		return nil
	}

	declared, err := ConfigFromJSON(config)

	if err != nil {
		return err
	}

	schema, err := SchemaFromJSON(config)

	if err != nil {
		return err
	}

	if server.catalog == nil {
		server.catalog = Open(ctx)
	}

	loaded, err := server.catalog.CreateTable(
		ctx, declared.Namespace, declared.Table, schema,
	)

	if err != nil {
		return err
	}

	server.opened = declared
	server.table = loaded

	return nil
}

/*
Done commits what is held and reports what reached the catalog.
*/
func (server *IcebergTableServer) Done(ctx context.Context, call IcebergTable_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to allocate results",
			err,
		))
	}

	if err := server.commit(ctx); err != nil {
		return err
	}

	// What the caller gets back is the state of the record, not the rows: the
	// table is where the rows went.
	report, err := sonic.Marshal(map[string]any{
		"table":     server.opened.Namespace + "." + server.opened.Table,
		"committed": server.committed,
		"pending":   server.held(),
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to encode what was committed",
			err,
		))
	}

	if err := results.SetOut(report); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to set out",
			err,
		))
	}

	return nil
}

/* held is how many rows have not reached the catalog. */
func (server *IcebergTableServer) held() int {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	return len(server.pending)
}

/*
commit sends what is held as successive snapshots, each small enough for the
catalog call to finish.

Rows are detached for the append and only the unacknowledged suffix is put
back. A snapshot that was already sent is not retried: repeating it could
duplicate it, and a duplicate frame in the record is worse than a gap that is
reported.
*/
func (server *IcebergTableServer) commit(ctx context.Context) error {
	server.mutex.Lock()
	rows := server.pending
	server.pending = nil
	table := server.table
	server.mutex.Unlock()

	if len(rows) == 0 {
		return nil
	}

	if table == nil {
		server.restore(rows)

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] rows were written before a table was declared",
			nil,
		))
	}

	sent := 0

	for sent < len(rows) {
		end, _, err := span(sent, len(rows), server.appendBytes, func(index int) int {
			return len(rows[index])
		})

		if err != nil {
			server.restore(rows[sent:])
			return err
		}

		acknowledged, err := server.appendRange(ctx, table, rows, sent, end)

		if err != nil {
			if acknowledged {
				sent = end
			}

			server.restore(rows[sent:])
			return err
		}

		sent = end
	}

	server.mutex.Lock()
	server.committed += int64(sent)
	server.mutex.Unlock()

	return nil
}

/* restore puts unacknowledged rows back at the front of what is held. */
func (server *IcebergTableServer) restore(rows [][]byte) {
	if len(rows) == 0 {
		return
	}

	server.mutex.Lock()
	server.pending = append(append([][]byte{}, rows...), server.pending...)
	server.mutex.Unlock()
}

/*
appendRange sends one snapshot. It reports whether the snapshot reached the
catalog, because a failure after sending is not the same as one before it.
*/
func (server *IcebergTableServer) appendRange(
	ctx context.Context, table *icetable.Table, rows [][]byte, start, end int,
) (bool, error) {
	reader, err := records(
		table.Schema(), end-start,
		func(index int) int { return len(rows[start+index]) },
		func(builder *array.RecordBuilder, from, to int) {
			for index := from; index < to; index++ {
				fillRow(builder, server.opened, rows[start+index])
			}
		},
	)

	if err != nil {
		return false, err
	}

	defer reader.Release()

	if _, err := table.Append(ctx, reader, nil); err != nil {
		return true, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to append to "+server.opened.Table,
			err,
		))
	}

	return true, nil
}

/*
fillRow writes one payload into the builder, column by declared column.

A field the payload does not carry is written as null rather than as a zero:
"not in this record" and "recorded as zero" are different readings and must
not share a representation.
*/
func fillRow(builder *array.RecordBuilder, declared TableConfig, payload []byte) {
	var held map[string]any

	if err := sonic.Unmarshal(payload, &held); err != nil {
		held = nil
	}

	columns := len(builder.Fields())

	for index, field := range declared.Fields {
		if index >= columns {
			break
		}

		value, carried := held[field.Name]

		if !carried {
			builder.Field(index).AppendNull()
			continue
		}

		appendValue(builder, index, value, payload)
	}
}

/* appendValue writes one field as whatever the column holds. */
func appendValue(
	builder *array.RecordBuilder, index int, value any, payload []byte,
) {
	switch target := builder.Field(index).(type) {
	case *array.StringBuilder:
		text, ok := value.(string)

		if !ok {
			target.AppendNull()
			return
		}

		target.Append(text)

	case *array.Int64Builder:
		number, ok := value.(float64)

		if !ok {
			target.AppendNull()
			return
		}

		target.Append(int64(number))

	case *array.Float64Builder:
		number, ok := value.(float64)

		if !ok {
			target.AppendNull()
			return
		}

		target.Append(number)

	case *array.BooleanBuilder:
		flag, ok := value.(bool)

		if !ok {
			target.AppendNull()
			return
		}

		target.Append(flag)

	case *array.TimestampBuilder:
		text, ok := value.(string)

		if !ok {
			target.AppendNull()
			return
		}

		instant, err := time.Parse(time.RFC3339Nano, text)

		if err != nil {
			target.AppendNull()
			return
		}

		timestamp(target, instant)

	case *array.BinaryBuilder:
		// A binary column carries the record exactly as it arrived, which is
		// what makes the tape replayable rather than merely summarised.
		target.Append(payload)

	default:
		builder.Field(index).AppendNull()
	}
}

/*
IcebergScanServer reads rows back out of an Iceberg table.

This is the other end of the record: what was captured is what gets replayed,
so a fragment comes back out of the same table the tape went into.
*/
type IcebergScanServer struct {
	catalog *Catalog
	rows    []byte
}

func NewIcebergScan() *IcebergScanServer {
	return &IcebergScanServer{}
}

/*
Write reads the declared table and holds what it carries.
*/
func (server *IcebergScanServer) Write(ctx context.Context, call IcebergScan_write) error {
	config, err := call.Args().Config()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[iceberg] failed to read the table declaration",
			err,
		))
	}

	if config == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] a table has to be declared before it can be read",
			nil,
		))
	}

	declared, err := ConfigFromJSON(config)

	if err != nil {
		return err
	}

	if server.catalog == nil {
		server.catalog = Open(ctx)
	}

	loaded, err := server.catalog.Load(ctx, declared.Namespace, declared.Table)

	if err != nil {
		return err
	}

	held, err := server.read(ctx, loaded, declared)

	if err != nil {
		return err
	}

	server.rows = held
	return nil
}

/*
read walks the table's current snapshot and collects its rows.
*/
func (server *IcebergScanServer) read(
	ctx context.Context, loaded *icetable.Table, declared TableConfig,
) ([]byte, error) {
	scan := loaded.Scan()
	_, batches, err := scan.ToArrowRecords(ctx)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to scan "+declared.Table,
			err,
		))
	}

	collected := make([]map[string]any, 0)

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.BadGateway,
				"[iceberg] failed to read a batch of "+declared.Table,
				err,
			))
		}

		for row := range int(batch.NumRows()) {
			held := make(map[string]any, batch.NumCols())

			for column := range int(batch.NumCols()) {
				name := batch.Schema().Field(column).Name
				held[name] = batch.Column(column).GetOneForMarshal(row)
			}

			collected = append(collected, held)
		}

		batch.Release()
	}

	encoded, err := sonic.Marshal(collected)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to encode what was read",
			err,
		))
	}

	return encoded, nil
}

/*
Done hands back what was read.
*/
func (server *IcebergScanServer) Done(ctx context.Context, call IcebergScan_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to allocate results",
			err,
		))
	}

	if len(server.rows) == 0 {
		return nil
	}

	if err := results.SetOut(server.rows); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to set out",
			err,
		))
	}

	server.rows = nil
	return nil
}
