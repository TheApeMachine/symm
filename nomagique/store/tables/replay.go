package tables

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"iter"
	"os"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/iceberg-go"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	_ "modernc.org/sqlite"
)

/*
Replay reconstructs complete workspace boundaries from all four tables.
Iceberg scan order is not event order. A temporary index orders references into
binary Arrow batches by recorded sequence and producer. Decoded batches remain
in memory only while they have unconsumed indexed rows.
No timestamp participates in ordering. A training publication seals its input
count; incomplete or duplicate boundaries fail rather than inventing history.
*/
func (catalog *Catalog) Replay(ctx context.Context, epoch int64, through ...int64) (
	[]ExcursionRecord, iter.Seq2[*data.Measurement[float64], error], error,
) {
	excursions, err := catalog.Excursions(ctx, epoch, nil)

	if err != nil {
		return nil, nil, errnie.Error(err)
	}

	frames := func(yield func(*data.Measurement[float64], error) bool) {
		directory, err := os.MkdirTemp("", "symm-replay-")

		if err != nil {
			yield(nil, errnie.Error(err))
			return
		}

		defer func() {
			if err := os.RemoveAll(directory); err != nil {
				errnie.Error(err)
			}
		}()

		database, err := sql.Open("sqlite", filepath.Join(directory, "tape.sqlite"))

		if err != nil {
			yield(nil, errnie.Error(err))
			return
		}

		defer func() {
			if err := database.Close(); err != nil {
				errnie.Error(err)
			}
		}()

		if err := catalog.indexTape(ctx, database, epoch, through...); err != nil {
			yield(nil, err)
			return
		}

		rows, err := database.QueryContext(ctx, "SELECT batch, ordinal FROM tape ORDER BY tick, owner")

		if err != nil {
			yield(nil, errnie.Error(err))
			return
		}

		defer func() {
			if err := rows.Close(); err != nil {
				errnie.Error(err)
			}
		}()

		frame := data.NewMeasurement[float64]("replay", nil)
		sealed := -1
		previousSequence := int64(0)

		// Retain each interleaved batch once, releasing it after its final indexed row.
		batches := make(map[int64]*replayBatch)
		defer func() {
			for _, batch := range batches {
				batch.Close()
			}
		}()

		for rows.Next() {
			var identity, ordinal int64

			if err := rows.Scan(&identity, &ordinal); err != nil {
				yield(nil, errnie.Error(err))
				return
			}
			batch := batches[identity]

			if batch == nil {
				batch = &replayBatch{}
				batches[identity] = batch
			}
			measurement, err := batch.Read(ctx, database, identity, ordinal)

			if err != nil {
				yield(nil, err)
				return
			}

			batch.remaining--

			if batch.remaining == 0 {
				batch.Close()
				delete(batches, identity)
			}

			if frame.SeqIdx != 0 && frame.SeqIdx != measurement.SeqIdx {
				if err := sealReplay(frame, sealed); err != nil {
					yield(nil, err)
					return
				}

				if !yield(frame, nil) {
					return
				}

				previousSequence = frame.SeqIdx
				frame = data.NewMeasurement[float64]("replay", nil)
				sealed = -1
			}

			frame.SeqIdx = measurement.SeqIdx

			if measurement.Source == "training" {
				previous, found := measurement.Metrics["previous_input"]

				if !found || previous.Raw != float64(previousSequence) {
					yield(nil, errnie.Error(errnie.Err(errnie.Validation, "replay: missing preceding workspace boundary", nil)))
					return
				}

				if measurement.Metrics["impulse_version"].Raw != 1.0 {
					yield(nil, errnie.Error(errnie.Err(errnie.Validation, "replay: unsupported impulse format", nil)))
					return
				}
				count, found := measurement.Metrics["input_count"]

				if !found {
					yield(nil, errnie.Error(errnie.Err(errnie.Validation, "replay: tape predates input boundary seals", nil)))
					return
				}

				sealed = int(count.Raw)
				continue
			}

			frame.Peers = append(frame.Peers, measurement)
		}

		if err := rows.Err(); err != nil {
			yield(nil, errnie.Error(err))
			return
		}

		if frame.SeqIdx == 0 {
			return
		}

		if err := sealReplay(frame, sealed); err != nil {
			yield(nil, err)
			return
		}

		yield(frame, nil)
	}
	return excursions, frames, nil
}

func sealReplay(frame *data.Measurement[float64], expected int) error {
	if expected < 0 || len(frame.Peers) != expected {
		return errnie.Error(errnie.Err(errnie.Validation, "replay: incomplete workspace input boundary", nil))
	}
	return nil
}

func (catalog *Catalog) indexTape(ctx context.Context, database *sql.DB, epoch int64, through ...int64) error {
	if _, err := database.ExecContext(ctx,
		"CREATE TABLE tape (tick INTEGER, owner TEXT, batch INTEGER, ordinal INTEGER, PRIMARY KEY(tick, owner)) WITHOUT ROWID",
	); err != nil {
		return errnie.Error(err)
	}

	if _, err := database.ExecContext(ctx, "CREATE TABLE batches (id INTEGER PRIMARY KEY, remaining INTEGER NOT NULL, payload BLOB NOT NULL)"); err != nil {
		return errnie.Error(err)
	}

	transaction, err := database.BeginTx(ctx, nil)

	if err != nil {
		return errnie.Error(err)
	}

	committed := false
	defer func() {
		if !committed {
			if err := transaction.Rollback(); err != nil {
				errnie.Error(err)
			}
		}
	}()

	statement, err := transaction.PrepareContext(ctx, "INSERT INTO tape VALUES (?, ?, ?, ?)")

	if err != nil {
		return errnie.Error(err)
	}

	defer func() {
		if err := statement.Close(); err != nil {
			errnie.Error(err)
		}
	}()

	var filter iceberg.BooleanExpression

	if len(through) > 0 {
		filter = iceberg.LessThanEqual(iceberg.Reference("tick"), through[0])
	}
	var payload bytes.Buffer
	for _, family := range []string{SpotTicker, SpotTrade, SpotLevel3, Measurements} {
		var indexed int64
		for batch, err := range catalog.scanRecords(ctx, family, epoch, filter, 0) {
			if err != nil {
				return errnie.Error(err)
			}
			indexed += batch.NumRows()
			payload.Reset()
			writer := ipc.NewWriter(&payload, ipc.WithSchema(batch.Schema()))

			if err := writer.Write(batch); err != nil {
				if closeErr := writer.Close(); closeErr != nil {
					errnie.Error(closeErr)
				}
				return errnie.Error(err)
			}

			if err := writer.Close(); err != nil {
				return errnie.Error(err)
			}
			inserted, err := transaction.ExecContext(ctx, "INSERT INTO batches(remaining, payload) VALUES (?, ?)", batch.NumRows(), payload.Bytes())

			if err != nil {
				return errnie.Error(err)
			}
			identity, err := inserted.LastInsertId()

			if err != nil {
				return errnie.Error(err)
			}
			ticks := batch.Column(batch.Schema().FieldIndices("tick")[0]).(*array.Int64)
			provenance := batch.Column(batch.Schema().FieldIndices("provenance")[0]).(*array.Map)
			keys, values := provenance.Keys().(*array.String), provenance.Items().(*array.String)
			for ordinal := range int(batch.NumRows()) {
				owner := ""
				start, end := provenance.ValueOffsets(ordinal)
				for offset := start; offset < end; offset++ {
					if keys.Value(int(offset)) == "owner" {
						owner = values.Value(int(offset))
						break
					}
				}

				if owner == "" || ticks.Value(ordinal) <= 0 {
					return errnie.Error(errnie.Err(errnie.Validation, "replay: tape has no producer identity or sequence", nil))
				}

				if _, err := statement.ExecContext(ctx, ticks.Value(ordinal), owner, identity, ordinal); err != nil {
					return errnie.Error(errnie.Err(errnie.Conflict, "replay: duplicate or unwritable input", err))
				}
			}
		}
		errnie.Info(fmt.Sprintf("replay: epoch %d indexed %s (%d rows)", epoch, family, indexed))
	}

	if err := transaction.Commit(); err != nil {
		return errnie.Error(err)
	}

	committed = true
	return nil
}
