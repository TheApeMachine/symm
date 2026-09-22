package data

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/theapemachine/errnie"
)

/* ArrowServer projects Arrow IPC records without interpreting their domain. */
type ArrowServer struct{ out []byte }

/* NewArrow constructs an idle projection primitive. */
func NewArrow() *ArrowServer { return &ArrowServer{} }

/* Write decodes the supplied IPC stream using its own schema. */
func (server *ArrowServer) Write(ctx context.Context, call Arrow_write) error {
	server.out = nil
	payload, err := call.Args().Data()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "arrow: data", err))
	}
	if len(payload) == 0 {
		return nil
	}
	reader, err := ipc.NewReader(bytes.NewReader(payload))
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "arrow: IPC stream", err))
	}
	defer reader.Release()
	remaining := call.Args().Row()
	for reader.Next() {
		batch := reader.RecordBatch()
		if remaining >= uint64(batch.NumRows()) {
			remaining -= uint64(batch.NumRows())
			continue
		}
		record := make(map[string]json.RawMessage, batch.NumCols())
		for index, column := range batch.Columns() {
			encoded, err := json.Marshal(column.GetOneForMarshal(int(remaining)))
			if err != nil {
				return errnie.Error(errnie.Err(errnie.Validation, "arrow: field", err))
			}
			record[batch.Schema().Field(index).Name] = encoded
		}
		server.out, err = json.Marshal(record)
		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "arrow: encode row", err))
		}
		return nil
	}
	if err := reader.Err(); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "arrow: read IPC", err))
	}
	return errnie.Error(errnie.Err(errnie.Validation, "arrow: row index exceeds IPC stream", nil))

}

/* Done emits the projected row and clears this evaluation's output. */
func (server *ArrowServer) Done(ctx context.Context, call Arrow_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "arrow: results", err))
	}
	if err := result.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "arrow: output", err))
	}
	server.out = nil
	return nil
}
