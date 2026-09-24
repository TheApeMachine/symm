package data

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/theapemachine/errnie"
)

/* ArrowServer projects Arrow IPC records without interpreting their domain. */
type ArrowServer struct{ rows [][]byte }

/* NewArrow constructs an idle projection primitive. */
func NewArrow() *ArrowServer { return &ArrowServer{} }

/* Write decodes the supplied IPC stream using its own schema. */
func (server *ArrowServer) Write(ctx context.Context, call Arrow_write) error {
	server.rows = server.rows[:0]
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

	for reader.Next() {
		batch := reader.RecordBatch()

		for row := range int(batch.NumRows()) {
			record := make(map[string]json.RawMessage, batch.NumCols())

			for index, column := range batch.Columns() {
				encoded, err := json.Marshal(column.GetOneForMarshal(row))

				if err != nil {
					return errnie.Error(errnie.Err(errnie.Validation, "arrow: field", err))
				}

				record[batch.Schema().Field(index).Name] = encoded
			}

			encoded, err := json.Marshal(record)

			if err != nil {
				return errnie.Error(errnie.Err(errnie.Internal, "arrow: encode row", err))
			}

			server.rows = append(server.rows, encoded)
		}
	}

	if err := reader.Err(); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "arrow: read IPC", err))
	}

	return nil
}

/* Done emits the projected rows and clears this evaluation's output. */
func (server *ArrowServer) Done(ctx context.Context, call Arrow_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "arrow: results", err))
	}

	rows, err := result.NewRows(int32(len(server.rows)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "arrow: rows", err))
	}

	for index, row := range server.rows {
		if err := rows.Set(index, row); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "arrow: row", err))
		}
	}

	server.rows = server.rows[:0]
	return nil
}
