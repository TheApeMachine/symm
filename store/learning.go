package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
)

const learningSchema = `
CREATE TABLE IF NOT EXISTS learning_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    symbol TEXT NOT NULL,
    at TEXT NOT NULL,
    data BLOB NOT NULL
) STRICT;
`

/*
EnsureLearningSchema creates the journal and announces each missing index before
SQLite scans retained rows. Completed indexes are reused on subsequent starts.
*/
func (store *SQLite) EnsureLearningSchema() error {
	if _, err := store.database.Exec(learningSchema); err != nil {
		return errnie.Err(errnie.IO, "store: ensure learning journal", err)
	}

	for _, index := range []struct{ name, columns string }{
		{"learning_events_candidate", "run_id, json_extract(data, '$.candidateId')"},
		{"learning_events_identity", "run_id, json_extract(data, '$.id'), json_extract(data, '$.kind')"},
		{"learning_events_kind", "json_extract(data, '$.kind'), id"},
		{"learning_events_run", "run_id, id"},
		{"learning_events_symbol", "run_id, symbol, id"},
	} {
		var exists bool

		if err := store.database.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='index' AND name=?)`,
			index.name,
		).Scan(&exists); err != nil {
			return errnie.Err(errnie.IO, "store: inspect learning index "+index.name, err)
		}

		if exists {
			continue
		}

		started := time.Now()
		errnie.Info("store: building learning index " + index.name +
			"; scanning retained learning journal (one-time migration)")

		if _, err := store.database.Exec("CREATE INDEX IF NOT EXISTS " + index.name +
			" ON learning_events(" + index.columns + ")"); err != nil {
			return errnie.Err(errnie.IO, "store: build learning index "+index.name, err)
		}

		errnie.Info(fmt.Sprintf("store: learning index %s ready after %s", index.name, time.Since(started)))
	}

	return nil
}

/*
WriteLearning records an internal learning event in the existing event journal.
It has a run identity but no invented external capture identity. Serialization
freezes the small decision record before it enters the shared ordered writer.
*/
func (writer *Writer) WriteLearning(runID hindsight.RunID, event hindsight.LearningEvent) error {
	if runID == "" || event.ID == 0 || event.At.IsZero() || event.Symbol == "" {
		return errnie.Err(errnie.Validation, "learning journal: run, decision, time and symbol are required", nil)
	}
	event.Run = runID
	payload, err := json.Marshal(event)

	if err != nil {
		return errnie.Err(errnie.Validation, fmt.Sprintf("learning journal: encode %s/%s decision %d for %s", event.Mode, event.Kind, event.ID, event.Symbol), err)
	}
	return writer.enqueue(writerOperation{kind: writerLearning, runID: runID, at: event.At, symbol: event.Symbol, payload: payload})
}

/* writeLearningOperation persists an internal event without disguising it as market input. */
func (store *SQLite) writeLearningOperation(executor sqliteExecutor, operation writerOperation) error {
	_, err := executor.Exec(`INSERT INTO learning_events(at, data, run_id, symbol)
		VALUES(?, ?, ?, ?)`,
		operation.at.UTC().Format(time.RFC3339Nano), operation.payload, string(operation.runID), operation.symbol)
	return err
}

/* LearningEvents reads a requested run's most recent immutable decision records. */
func (store *SQLite) LearningEvents(runID hindsight.RunID, symbol, candidate string, limit int) ([]hindsight.LearningEvent, error) {
	if limit <= 0 {
		return nil, errnie.Err(errnie.Validation, "learning journal: positive page size required", nil)
	}
	query := `SELECT data FROM learning_events WHERE run_id=? ORDER BY id DESC LIMIT ?`
	arguments := []any{string(runID), limit}

	if symbol != "" {
		query = `SELECT data FROM learning_events WHERE run_id=? AND symbol=? ORDER BY id DESC LIMIT ?`
		arguments = []any{string(runID), symbol, limit}
	}

	if candidate != "" {
		query = `SELECT data FROM learning_events WHERE run_id=? AND json_extract(data,'$.candidateId')=? ORDER BY id DESC LIMIT ?`
		arguments = []any{string(runID), candidate, limit}
	}
	rows, err := store.reader.Query(query, arguments...)

	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]hindsight.LearningEvent, 0)

	for rows.Next() {
		var payload []byte

		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var event hindsight.LearningEvent

		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

/*
	LearningExperiences loads complete issue/resolve experiences across producer runs.

The limit counts resolved experiences, never raw rows that might cut off their
issues. Run IDs come from the indexed storage identity, including legacy JSON.
*/
func (store *SQLite) LearningExperiences(kind string, limit int) ([]hindsight.LearningEvent, error) {
	if limit <= 0 || (kind != "resolved" && kind != "portfolio_resolved") {
		return nil, errnie.Err(errnie.Validation, "learning journal: resolved experience kind and positive retention required", nil)
	}
	issuedKind := "issued"

	if kind == "portfolio_resolved" {
		issuedKind = "portfolio_issued"
	}
	query := `WITH pairs AS (
 SELECT resolved.id AS ordinal, resolved.run_id AS run, issued.data AS issued, resolved.data AS resolved
 FROM learning_events AS resolved JOIN learning_events AS issued
 ON issued.run_id=resolved.run_id AND json_extract(issued.data,'$.id')=json_extract(resolved.data,'$.id')
 AND json_extract(issued.data,'$.kind')=?
 AND json_extract(issued.data,'$.mode') IS json_extract(resolved.data,'$.mode')
 WHERE json_extract(resolved.data,'$.kind')=?
 ORDER BY resolved.id DESC LIMIT ?
 ) SELECT run, issued, resolved FROM pairs ORDER BY ordinal`
	rows, err := store.reader.Query(query, issuedKind, kind, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]hindsight.LearningEvent, 0)
	for rows.Next() {
		var run hindsight.RunID
		var issued, resolved []byte

		if err := rows.Scan(&run, &issued, &resolved); err != nil {
			return nil, err
		}
		for _, payload := range [][]byte{issued, resolved} {
			var event hindsight.LearningEvent

			if err := json.Unmarshal(payload, &event); err != nil {
				return nil, err
			}
			event.Run = run
			events = append(events, event)
		}
	}
	return events, rows.Err()
}
