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
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"

	// Registers the s3:// file IO the warehouse stores its metadata and data files behind.
	_ "github.com/apache/iceberg-go/io/gocloud"
)

/*
IcebergTableServer appends what it is written to an Iceberg table.

Every append is a snapshot plus a metadata write, so rows are held until the
node is read rather than written one at a time. Reads dispatch one append
worker without waiting for catalog I/O. What is held is what has not been
acknowledged: a failed commit retains its rows and blocks admission until an
explicit Flush retries it. Rows are never dropped to keep the node moving — a tape with
a hole in it cannot be replayed against.

The table is declared by config using Iceberg field types as JSON. What the columns are
and what they are called is the caller's declaration, not this node's
knowledge.
*/
type IcebergTableServer struct {
	mutex           sync.Mutex
	io              sync.Mutex
	working         chan struct{}
	failure         error
	inflight        int
	maxPendingBytes int
	Catalog         catalog.Catalog
	opened          struct {
		Namespace       string                `json:"namespace"`
		Table           string                `json:"table"`
		Catalog         string                `json:"catalog"`
		Properties      iceberg.Properties    `json:"properties"`
		Fields          []iceberg.NestedField `json:"fields"`
		AppendBytes     int                   `json:"appendBytes"`
		MaxPendingBytes int                   `json:"maxPendingBytes"`
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

	if err := server.configure(config); err != nil {
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

	if server.failure != nil {
		return server.failure
	}
	arrivals := make([][]byte, 0, rows.Len()+1)
	if len(payload) > 0 {
		arrivals = append(arrivals, payload)
	}
	size := len(payload)
	for index := range rows.Len() {
		row, err := rows.At(index)
		if err != nil {
			return errnie.Error(err)
		}
		if len(row) == 0 {
			continue
		}
		size += len(row)
		arrivals = append(arrivals, row)
	}
	if size > server.maxPendingBytes-server.held {
		return errnie.Error(errnie.Err(errnie.IO, "iceberg: pending byte budget exhausted; batch was not admitted", nil))
	}
	for _, row := range arrivals {
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
	server.mutex.Lock()
	working := server.working
	server.mutex.Unlock()
	if working != nil {
		select {
		case <-working:
		case <-ctx.Done():
			return errnie.Error(ctx.Err())
		}
	}
	err := server.commit(ctx)
	server.mutex.Lock()
	server.failure = err
	server.mutex.Unlock()
	return err
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
configure validates the immutable declaration without accessing external storage.
*/
func (server *IcebergTableServer) configure(config string) error {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	if server.declaration == config {
		return nil
	}

	if server.declaration != "" {
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
	server.maxPendingBytes = server.opened.MaxPendingBytes
	// Two append buffers permit one batch in flight while the next fills.
	if server.maxPendingBytes == 0 {
		server.maxPendingBytes = 2 * server.appendBytes
	}
	if server.maxPendingBytes < server.appendBytes {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: pending byte budget is smaller than an append", nil))
	}
	server.declaration = config
	return nil
}

/* open resolves the catalog only on this node's serialized persistence worker. */
func (server *IcebergTableServer) open(ctx context.Context) error {
	if server.table != nil {
		return nil
	}
	if server.declaration == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "iceberg: table has not been declared", nil))
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

	return nil
}

/*
Done dispatches eligible writes without waiting and reports acknowledged progress.
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
		server.dispatch()
	}
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if server.failure != nil {
		return server.failure
	}
	results.SetPending(int64(len(server.pending) + server.inflight))
	results.SetCommitted(server.committed)
	results.SetBytes(int64(server.held))
	return errnie.Error(results.SetTable(server.opened.Namespace + "." + server.opened.Table))
}

/* dispatch starts at most one append worker and never waits for external I/O. */
func (server *IcebergTableServer) dispatch() {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if server.working != nil || server.failure != nil {
		return
	}
	finished := make(chan struct{})
	server.working = finished
	server.asked = false
	go func() {
		err := server.commit(context.Background())
		server.mutex.Lock()
		server.failure = err
		server.working = nil
		close(finished)
		server.mutex.Unlock()
	}()
}

/* Shutdown waits for admitted I/O and makes an unflushed tail explicit. */
func (server *IcebergTableServer) Shutdown() {
	server.mutex.Lock()
	working := server.working
	server.mutex.Unlock()
	if working != nil {
		<-working
	}
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if server.held > 0 {
		errnie.Error(errnie.Err(errnie.IO, "iceberg: capability released with unflushed rows", server.failure))
	}
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
	server.io.Lock()
	defer server.io.Unlock()
	server.mutex.Lock()
	empty := len(server.pending) == 0
	server.mutex.Unlock()
	if empty {
		return nil
	}
	if err := server.open(ctx); err != nil {
		return err
	}
	server.mutex.Lock()
	rows := server.pending
	server.pending = nil
	server.inflight = len(rows)
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
		server.inflight -= end - sent
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
	server.inflight = 0
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
