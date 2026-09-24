package tables

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	_ "github.com/apache/iceberg-go/catalog/rest"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"

	// Registers the s3:// file IO the warehouse stores its metadata and data files behind.
	_ "github.com/apache/iceberg-go/io/gocloud"
)

/*
IcebergTableServer appends what it is written to an Iceberg table.

Every append is a snapshot plus a metadata write, so rows are held until the
node is read rather than written one at a time. What is held is what has not
been acknowledged: a commit that fails leaves its rows here, and the next read
sends them again. Rows are never dropped to keep the node moving — a tape with
a hole in it cannot be replayed against.

The table is declared by config using Iceberg field types as JSON. What the columns are
and what they are called is the caller's declaration, not this node's
knowledge.
*/
type IcebergTableServer struct {
	mutex   sync.Mutex
	Catalog catalog.Catalog
	opened  struct {
		Namespace   string                `json:"namespace"`
		Table       string                `json:"table"`
		Catalog     string                `json:"catalog"`
		Properties  iceberg.Properties    `json:"properties"`
		Fields      []iceberg.NestedField `json:"fields"`
		AppendBytes int                   `json:"appendBytes"`
	}
	declaration string
	table       *icetable.Table

	pending     [][]byte
	held        int
	appendBytes int
	committed   int64
	asked       bool
}

func NewIcebergTable() *IcebergTableServer {
	return &IcebergTableServer{
		// Arrow binary offsets are signed 32-bit; a batch cannot exceed them.
		appendBytes: math.MaxInt32,
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

	rows, err := call.Args().Rows()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.BadRequest, "[iceberg] failed to read the rows", err))
	}

	server.hold(payload)

	for index := range rows.Len() {
		row, err := rows.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.BadRequest, "[iceberg] failed to read a row", err))
		}

		server.hold(row)
	}

	return nil
}

/* hold keeps one row until the next commit; an empty row is nothing written. */
func (server *IcebergTableServer) hold(row []byte) {
	if len(row) == 0 {
		return
	}

	server.pending = append(server.pending, bytes.Clone(row))
	server.held += len(row)
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

	if server.table != nil && server.declaration == config {
		return nil
	}

	if server.table != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: cannot change the declaration of an open table", nil))
	}
	if err := json.Unmarshal([]byte(config), &server.opened); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: invalid table declaration", err))
	}
	if server.opened.Namespace == "" || server.opened.Table == "" || len(server.opened.Fields) == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: namespace, table and fields are required", nil))
	}
	if server.opened.AppendBytes < 0 || server.opened.AppendBytes > math.MaxInt32 {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: appendBytes exceeds Arrow's binary offset domain", nil))
	}
	if server.opened.AppendBytes > 0 {
		server.appendBytes = server.opened.AppendBytes
	}
	var err error
	if server.Catalog == nil {
		server.Catalog, err = catalog.Load(ctx, server.opened.Catalog, server.opened.Properties)
		if err != nil {
			return errnie.Error(errnie.Err(errnie.IO, "iceberg: open catalog", err))
		}
	}
	namespace := strings.Split(server.opened.Namespace, ".")
	if err := server.Catalog.CreateNamespace(ctx, namespace, nil); err != nil && !errors.Is(err, catalog.ErrNamespaceAlreadyExists) {
		return errnie.Error(errnie.Err(errnie.IO, "iceberg: create namespace", err))
	}
	identifier := append(namespace, server.opened.Table)
	loaded, err := server.Catalog.LoadTable(ctx, identifier)
	if errors.Is(err, catalog.ErrNoSuchTable) {
		loaded, err = server.Catalog.CreateTable(ctx, identifier, iceberg.NewSchema(0, server.opened.Fields...))
	}
	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "iceberg: open table", err))
	}
	if !loaded.Schema().Equals(iceberg.NewSchema(loaded.Schema().ID, server.opened.Fields...)) {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: declaration differs from stored schema", nil))
	}
	server.table = loaded
	server.declaration = config

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
		end, held := sent, 0
		for end < len(rows) && len(rows[end]) <= server.appendBytes-held {
			held += len(rows[end])
			end++
		}
		if end == sent {
			server.restore(rows[sent:])
			return errnie.Error(errnie.Err(errnie.Validation, "iceberg: row exceeds declared append byte budget", nil))
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
	converted, err := icetable.SchemaToArrowSchema(table.Schema(), nil, true, false)
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: Arrow schema", err))
	}
	builder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
	defer builder.Release()
	for _, row := range rows[start:end] {
		var values map[string]json.RawMessage
		if err := json.Unmarshal(row, &values); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "iceberg: invalid row", err))
		}
		for index, field := range converted.Fields() {
			value, exists := values[field.Name]
			if !exists || bytes.Equal(value, []byte("null")) {
				builder.Field(index).AppendNull()
				continue
			}
			if timestamp, ok := builder.Field(index).(*array.TimestampBuilder); ok {
				var instant time.Time
				if err := json.Unmarshal(value, &instant); err != nil {
					return errnie.Error(errnie.Err(errnie.Validation, "iceberg: invalid timestamp "+field.Name, err))
				}
				converted, err := arrow.TimestampFromTime(instant, timestamp.Type().(*arrow.TimestampType).Unit)
				if err != nil {
					return errnie.Error(errnie.Err(errnie.Validation, "iceberg: timestamp conversion", err))
				}
				timestamp.Append(converted)
				continue
			}
			if err := builder.Field(index).UnmarshalJSON(append(append([]byte("["), value...), ']')); err != nil {
				return errnie.Error(errnie.Err(errnie.Validation, "iceberg: invalid column "+field.Name, err))
			}
		}
	}
	record := builder.NewRecordBatch()
	defer record.Release()
	for index, field := range converted.Fields() {
		if !field.Nullable && record.Column(index).NullN() > 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "iceberg: missing required column "+field.Name, nil))
		}
	}
	reader, err := array.NewRecordReader(converted, []arrow.RecordBatch{record})
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: record reader", err))
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
