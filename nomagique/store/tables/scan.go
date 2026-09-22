package tables

import (
	"bytes"
	"context"
	"encoding/json"
	"iter"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/iceberg-go"
	icebergio "github.com/apache/iceberg-go/io"
	"github.com/apache/iceberg-go/table"
	"github.com/theapemachine/errnie"
)

/*
	IcebergScanServer reads a pinned snapshot. Its graph supplies loaded metadata;

this node neither opens a catalog nor loads a table declaration nor projects JSON.
*/
type IcebergScanServer struct {
	metadata, properties []byte
	next                 func() (arrow.RecordBatch, error, bool)
	stop                 func()
	batch                arrow.RecordBatch
	row                  int64
	exhausted            bool
}

/* NewIcebergScan constructs an idle scan primitive. */
func NewIcebergScan() *IcebergScanServer { return &IcebergScanServer{} }

/* Write accepts an already-loaded snapshot and its explicit file I/O properties. */
func (server *IcebergScanServer) Write(ctx context.Context, call IcebergScan_write) error {
	arrivals, err := call.Args().Metadata()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "scan: metadata", err))
	}
	if arrivals.Len() == 0 {
		return nil
	}
	if arrivals.Len() != 1 {
		return errnie.Error(errnie.Err(errnie.Validation, "scan: one snapshot is required", nil))
	}
	metadata, err := arrivals.At(0)
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "scan: metadata input", err))
	}
	properties, err := call.Args().Properties()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "scan: properties", err))
	}
	if server.next != nil {
		if !bytes.Equal(metadata, server.metadata) || !bytes.Equal(properties, server.properties) {
			return errnie.Error(errnie.Err(errnie.Validation, "scan: snapshot inputs changed during replay", nil))
		}
		return nil
	}
	snapshot, err := table.ParseMetadataBytes(metadata)
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "scan: invalid Iceberg metadata", err))
	}
	var settings iceberg.Properties
	if err := json.Unmarshal(properties, &settings); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "scan: explicit I/O properties object is required", err))
	}
	if settings == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "scan: I/O properties must be an object, not null", nil))
	}
	source := table.New(nil, snapshot, "", icebergio.LoadFSFunc(settings, snapshot.Location()), nil)
	_, batches, err := source.Scan().ToArrowRecords(ctx)
	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "scan: snapshot records", err))
	}
	server.next, server.stop = iter.Pull2(batches)
	server.metadata, server.properties = bytes.Clone(metadata), bytes.Clone(properties)
	return nil
}

/* Done emits one Arrow row as IPC so downstream graph evaluations remain ordered. */
func (server *IcebergScanServer) Done(ctx context.Context, call IcebergScan_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "scan: results", err))
	}
	if server.next == nil {
		result.SetIdle()
		return nil
	}
	if server.batch != nil && server.row == server.batch.NumRows() {
		server.batch.Release()
		server.batch = nil
	}
	for server.batch == nil && !server.exhausted {
		batch, err, available := server.next()
		if err != nil {
			return errnie.Error(errnie.Err(errnie.IO, "scan: read", err))
		}
		if !available {
			server.exhausted = true
			server.stop()
			break
		}
		if batch.NumRows() == 0 {
			batch.Release()
			continue
		}
		server.batch, server.row = batch, 0
	}
	result.SetExhausted(server.exhausted)
	if server.exhausted {
		return nil
	}
	row := server.batch.NewSlice(server.row, server.row+1)
	defer row.Release()
	var buffer bytes.Buffer
	writer := ipc.NewWriter(&buffer, ipc.WithSchema(row.Schema()))
	if err := writer.Write(row); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "scan: encode Arrow row", err))
	}
	if err := writer.Close(); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "scan: close IPC stream", err))
	}
	if err := result.SetOut(buffer.Bytes()); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "scan: output", err))
	}
	server.row++
	return nil
}

/* Shutdown releases this node's snapshot read resources. */
func (server *IcebergScanServer) Shutdown() {
	if server.batch != nil {
		server.batch.Release()
		server.batch = nil
	}
	if server.stop != nil {
		server.stop()
	}
}
