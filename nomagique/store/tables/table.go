package tables

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow/array"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
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
	held        int
	appendBytes int
	committed   int64
	asked       bool
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

	server.mutex.Lock()
	defer server.mutex.Unlock()

	if call.Args().Commit() {
		server.asked = true
	}

	if len(payload) == 0 {
		return nil
	}
	server.pending = append(server.pending, bytes.Clone(payload))
	server.held += len(payload)

	return nil
}

/* Flush persists the tail even when it has not reached the append byte budget. */
func (server *IcebergTableServer) Flush(ctx context.Context, call runtime.Durable_flush) error {
	return server.commit(ctx)
}

/*
worthSending reports whether there is a reason to spend a snapshot.

The reason is the storage, not the market: a snapshot carries a metadata
write whatever it holds, so one per observation makes the catalog the clock.
Rows wait until they add up to what an append is sized for, or until a caller
says now.
*/
func (server *IcebergTableServer) worthSending() bool {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	if server.asked {
		return true
	}

	return server.appendBytes > 0 && server.held >= server.appendBytes
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

	if server.catalog == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: catalog is not configured", nil))
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

	if server.worthSending() {
		if err := server.commit(ctx); err != nil {
			return err
		}
	}

	server.mutex.Lock()
	pending, committed, held := len(server.pending), server.committed, server.held
	server.asked = false
	server.mutex.Unlock()

	results.SetPending(int64(pending))
	results.SetCommitted(committed)
	results.SetBytes(int64(held))

	// What the caller gets back is the state of the record, not the rows: the
	// table is where the rows went.
	report, err := sonic.Marshal(map[string]any{
		"table":     server.opened.Namespace + "." + server.opened.Table,
		"committed": committed,
		"pending":   pending,
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

/*
commit sends what is held as successive snapshots, each small enough for the
catalog call to finish.

Rows are detached for the append and put back when it fails. A failed append
is ambiguous — the snapshot may or may not have reached the catalog — and
nothing here can tell the two apart. The rows are kept.

That risks a duplicate. It is the right risk to take: a duplicated
observation is repairable by whatever reads the table, and a missing one is
not. A tape with a hole in it cannot be replayed against, and nothing
downstream can tell a hole from a quiet market.
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

		if err := server.appendRange(ctx, table, rows, sent, end); err != nil {
			server.restore(rows[sent:])
			return err
		}

		server.mutex.Lock()
		table = server.table
		server.committed += int64(end - sent)
		for _, row := range rows[sent:end] {
			server.held -= len(row)
		}
		server.mutex.Unlock()

		sent = end
	}

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
appendRange sends one snapshot.
*/
func (server *IcebergTableServer) appendRange(
	ctx context.Context, table *icetable.Table, rows [][]byte, start, end int,
) error {
	reader, err := records(
		table.Schema(), end-start,
		func(index int) int { return len(rows[start+index]) },
		func(builder *array.RecordBuilder, from, to int) error {
			for index := from; index < to; index++ {
				if err := fillRow(builder, server.opened, rows[start+index]); err != nil {
					return err
				}
			}
			return nil
		},
	)

	if err != nil {
		return err
	}

	defer reader.Release()

	updated, err := table.Append(ctx, reader, nil)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to append to "+server.opened.Table,
			err,
		))
	}

	server.mutex.Lock()
	server.table = updated
	server.mutex.Unlock()

	return nil
}

/*
fillRow writes one payload into the builder, column by declared column.

A field the payload does not carry is written as null rather than as a zero:
"not in this record" and "recorded as zero" are different readings and must
not share a representation.
*/
func fillRow(builder *array.RecordBuilder, declared TableConfig, payload []byte) error {
	var held map[string]json.RawMessage

	if err := json.Unmarshal(payload, &held); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: row must be a JSON object", err))
	}

	if len(declared.Fields) != len(builder.Fields()) {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: declared columns differ from table schema", nil))
	}

	for index, field := range declared.Fields {
		if builder.Schema().Field(index).Name != field.Name {
			return errnie.Error(errnie.Err(errnie.Validation, "iceberg: declared column order differs from table schema", nil))
		}

		value, carried := held[field.Name]

		if !carried || bytes.Equal(value, []byte("null")) {
			if field.Required || !builder.Schema().Field(index).Nullable {
				return errnie.Error(errnie.Err(errnie.Validation, "iceberg: missing required column "+field.Name, nil))
			}

			builder.Field(index).AppendNull()
			continue
		}

		if err := appendValue(builder, index, value); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "iceberg: invalid column "+field.Name, err))
		}
	}

	return nil
}

/* appendValue decodes the declared field, never substitutes the entire input row. */
func appendValue(builder *array.RecordBuilder, index int, value json.RawMessage) error {
	switch target := builder.Field(index).(type) {
	case *array.StringBuilder:
		var text string

		if err := json.Unmarshal(value, &text); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "expected text", err))
		}

		target.Append(text)
	case *array.Int64Builder:
		var number int64

		if err := json.Unmarshal(value, &number); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "expected int64", err))
		}

		target.Append(number)
	case *array.Float64Builder:
		var number float64

		if err := json.Unmarshal(value, &number); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "expected float64", err))
		}

		target.Append(number)
	case *array.BooleanBuilder:
		var flag bool

		if err := json.Unmarshal(value, &flag); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "expected boolean", err))
		}

		target.Append(flag)
	case *array.TimestampBuilder:
		var instant time.Time

		if err := json.Unmarshal(value, &instant); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "expected RFC3339 timestamp", err))
		}

		timestamp(target, instant)
	case *array.BinaryBuilder:
		var binary []byte

		if err := json.Unmarshal(value, &binary); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "expected base64 binary", err))
		}

		target.Append(binary)
	default:
		return errnie.Error(errnie.Err(errnie.Validation, fmt.Sprintf("iceberg: unsupported column type %s", target.Type()), nil))
	}

	return nil
}

/*
IcebergScanServer reads rows back out of an Iceberg table.

This is the other end of the record: what was captured is what gets replayed,
so a fragment comes back out of the same table the tape went into.
*/
type IcebergScanServer struct {
	catalog  *Catalog
	payloads [][]byte
	cursor   int
	loaded   string
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

	if server.catalog == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: catalog is not configured", nil))
	}

	loaded, err := server.catalog.Load(ctx, declared.Namespace, declared.Table)

	if err != nil {
		return err
	}

	// The table is read once and then handed back a frame at a time. Reading
	// it again on every evaluation would replay the beginning forever.
	if server.loaded == declared.declaration {
		return nil
	}

	held, err := server.read(ctx, loaded, declared)

	if err != nil {
		return err
	}

	server.payloads = held
	server.cursor = 0
	server.loaded = declared.declaration

	return nil
}

/*
read walks the table's current snapshot and collects the frames it holds.

What comes back is what went in: the payload column carries each frame
exactly as it arrived, so replaying the table is the same thing happening
again rather than a summary of it being described. Anything that reads a
replayed frame cannot tell it from a live one, which is the only way a
decision made against the archive means anything about the market.
*/
func (server *IcebergScanServer) read(
	ctx context.Context, loaded *icetable.Table, declared TableConfig,
) ([][]byte, error) {
	scan := loaded.Scan()
	_, batches, err := scan.ToArrowRecords(ctx)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to scan "+declared.Table,
			err,
		))
	}

	collected := make([][]byte, 0)

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.BadGateway,
				"[iceberg] failed to read a batch of "+declared.Table,
				err,
			))
		}

		column := -1

		for index := range int(batch.NumCols()) {
			if batch.Schema().Field(index).Name == payloadColumn {
				column = index
				break
			}
		}

		if column < 0 {
			batch.Release()

			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[iceberg] "+declared.Table+" holds no "+payloadColumn+
					" column, so there is nothing to replay",
				nil,
			))
		}

		for row := range int(batch.NumRows()) {
			frame, carried := frameBytes(
				batch.Column(column).GetOneForMarshal(row),
			)

			// A row whose frame is missing is not an empty frame. Replaying
			// it as one would put a reading into the tape that never
			// happened.
			if !carried {
				batch.Release()
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: replay row has missing or malformed payload", nil))
			}

			collected = append(collected, frame)
		}

		batch.Release()
	}

	return collected, nil
}

// payloadColumn is where a frame is kept, exactly as it arrived.
const payloadColumn = "payload"

/*
frameBytes reads one archived frame, whichever way Arrow handed it over.
*/
func frameBytes(held any) ([]byte, bool) {
	switch value := held.(type) {
	case []byte:
		if len(value) == 0 {
			return nil, false
		}

		return value, true

	case string:
		if value == "" {
			return nil, false
		}

		decoded, err := base64.StdEncoding.DecodeString(value)

		if err == nil {
			return decoded, true
		}

		return nil, false
	}

	return nil, false
}

/*
Done hands back the next frame.

One frame per evaluation, in table scan order. Callers must establish capture
ordering before treating a multi-file scan as a chronological tape. Handing back the whole table at once would present
an array where every reader downstream expects one record.
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

	if server.cursor >= len(server.payloads) {
		return nil
	}

	frame := server.payloads[server.cursor]
	server.cursor++

	if err := results.SetOut(frame); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to set out",
			err,
		))
	}

	return nil
}
