package store

import (
	"encoding/json"
	"math"
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
CREATE INDEX IF NOT EXISTS learning_events_run ON learning_events(run_id, id);
CREATE INDEX IF NOT EXISTS learning_events_symbol ON learning_events(run_id, symbol, id);
`

/*
WriteLearning records an internal learning event in the existing event journal.
It has a run identity but no invented external capture identity. Serialization
freezes the small decision record before it enters the shared ordered writer.
*/
func (writer *Writer) WriteLearning(runID hindsight.RunID, event hindsight.LearningEvent) error {
	if runID == "" || event.ID == 0 || event.At.IsZero() || event.Symbol == "" {
		return errnie.Err(errnie.Validation, "learning journal: run, decision, time and symbol are required", nil)
	}
	sanitizeLearningFloats(&event)
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return writer.enqueue(writerOperation{kind: writerLearning, runID: runID, at: event.At, symbol: event.Symbol, payload: payload})
}

func sanitizeLearningFloats(event *hindsight.LearningEvent) {
	sanitizeFloat(&event.Authority)
	sanitizeFloat(&event.Target)
	sanitizeFloat(&event.Profit)
	sanitizeFloat(&event.Prior.Mean)
	sanitizeFloat(&event.Prior.Variance)
	sanitizeFloat(&event.Prior.Support)
	sanitizeFloat(&event.Prior.Maturity)
	sanitizeFloat(&event.Prior.Authority)
	sanitizeFloat(&event.Prior.Memory)
}

func sanitizeFloat(val *float64) {
	if math.IsNaN(*val) || math.IsInf(*val, 0) {
		*val = 0
	}
}

/* writeLearningOperation persists an internal event without disguising it as market input. */
func (store *SQLite) writeLearningOperation(executor sqliteExecutor, operation writerOperation) error {
	_, err := executor.Exec(`INSERT INTO learning_events(at, data, run_id, symbol)
		VALUES(?, ?, ?, ?)`,
		operation.at.UTC().Format(time.RFC3339Nano), operation.payload, string(operation.runID), operation.symbol)
	return err
}

/* LearningEvents reads a requested run's most recent immutable decision records. */
func (store *SQLite) LearningEvents(runID hindsight.RunID, symbol string, limit int) ([]hindsight.LearningEvent, error) {
	if limit <= 0 {
		return nil, errnie.Err(errnie.Validation, "learning journal: positive page size required", nil)
	}
	query := `SELECT data FROM learning_events WHERE run_id=? ORDER BY id DESC LIMIT ?`
	arguments := []any{string(runID), limit}

	if symbol != "" {
		query = `SELECT data FROM learning_events WHERE run_id=? AND symbol=? ORDER BY id DESC LIMIT ?`
		arguments = []any{string(runID), symbol, limit}
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

/* RecentLearningEvents reads recent journal decision records across all runs to warm up models on restart. */
func (store *SQLite) RecentLearningEvents(limit int) ([]hindsight.LearningEvent, error) {
	if limit <= 0 {
		return nil, errnie.Err(errnie.Validation, "learning journal: positive page size required", nil)
	}
	query := `SELECT data FROM (SELECT id, data FROM learning_events ORDER BY id DESC LIMIT ?) ORDER BY id ASC`
	rows, err := store.reader.Query(query, limit)
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
