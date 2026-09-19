package tables

import (
	"bytes"
	"context"
	"database/sql"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

// replayBatch retains one indexed Arrow batch while its rows are replayed.
// Metrics, decimal provenance and observation failures use the storage schema.
type replayBatch struct {
	identity  int64
	remaining int64
	record    arrow.RecordBatch
}

func (batch *replayBatch) Read(ctx context.Context, database *sql.DB, identity, ordinal int64) (*data.Measurement[float64], error) {
	if batch.record == nil || batch.identity != identity {
		var payload []byte

		if err := database.QueryRowContext(ctx, "SELECT payload, remaining FROM batches WHERE id = ?", identity).Scan(&payload, &batch.remaining); err != nil {
			return nil, errnie.Error(err)
		}
		reader, err := ipc.NewReader(bytes.NewReader(payload))

		if err != nil {
			return nil, errnie.Error(err)
		}
		defer reader.Release()

		if !reader.Next() {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "replay: indexed Arrow batch is empty or unreadable", reader.Err()))
		}
		batch.Close()
		batch.record = reader.RecordBatch()
		batch.record.Retain()
		batch.identity = identity
	}

	if ordinal < 0 || ordinal >= batch.record.NumRows() {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "replay: row outside indexed Arrow batch", nil))
	}
	row := batch.record.NewSlice(ordinal, ordinal+1)
	defer row.Release()
	measurements, err := readMeasurements(row)

	if err != nil {
		return nil, err
	}
	return measurements[0], nil
}

func (batch *replayBatch) Close() {
	if batch.record != nil {
		batch.record.Release()
		batch.record = nil
	}
}
