package tables

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/apache/iceberg-go"
	"iter"
	"os"
	"path/filepath"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	_ "modernc.org/sqlite"
)

// replayRow preserves an observation failure through the temporary JSON index.
type replayRow struct {
	Measurement *data.Measurement[float64]
	Error       string
}

/*
Replay reconstructs complete workspace boundaries from all four tables.
Iceberg scan order is not event order. An on-disk temporary index orders rows
by recorded sequence and producer, without retaining the epoch in Go memory.
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

		rows, err := database.QueryContext(ctx, "SELECT payload FROM tape ORDER BY tick, owner")

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

		for rows.Next() {
			var payload []byte

			if err := rows.Scan(&payload); err != nil {
				yield(nil, errnie.Error(err))
				return
			}

			var row replayRow

			if err := json.Unmarshal(payload, &row); err != nil {
				yield(nil, errnie.Error(err))
				return
			}

			measurement := row.Measurement
			if row.Error != "" {
				measurement.Err = errors.New(row.Error)
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

				if measurement.Metrics["impulse_version"].Raw != grid.FormatVersion {
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
		"CREATE TABLE tape (tick INTEGER, owner TEXT, payload BLOB, PRIMARY KEY(tick, owner)) WITHOUT ROWID",
	); err != nil {
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

	statement, err := transaction.PrepareContext(ctx, "INSERT INTO tape VALUES (?, ?, ?)")

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
	for _, family := range []string{SpotTicker, SpotTrade, SpotLevel3, Measurements} {
		for measurement, err := range catalog.scan(ctx, family, epoch, filter, 0) {
			if err != nil {
				return errnie.Error(err)
			}

			owner := measurement.Provenance["owner"]

			if owner == "" || measurement.SeqIdx <= 0 {
				return errnie.Error(errnie.Err(errnie.Validation, "replay: tape has no producer identity or sequence", nil))
			}

			row := replayRow{Measurement: measurement}
			if measurement.Err != nil {
				row.Error = measurement.Err.Error()
			}
			payload, err := json.Marshal(row)

			if err != nil {
				return errnie.Error(err)
			}

			if _, err := statement.ExecContext(ctx, measurement.SeqIdx, owner, payload); err != nil {
				return errnie.Error(errnie.Err(errnie.Conflict, "replay: duplicate or unwritable input", err))
			}
		}
	}

	if err := transaction.Commit(); err != nil {
		return errnie.Error(err)
	}

	committed = true
	return nil
}
