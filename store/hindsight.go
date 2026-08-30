package store

import (
	"encoding/json"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
)

/*
CaptureEntry is one row of the capture tape as the inspection UI lists it: the
frame's exact CaptureIdentity plus the transport facts that name where it came
from. It is the hindsight.CaptureIdentity and the events-table columns that
accompany it — not a parallel model.
*/
type CaptureEntry struct {
	Identity   hindsight.CaptureIdentity `json:"identity"`
	Kind       string                    `json:"kind"`
	Endpoint   string                    `json:"endpoint"`
	ReceivedAt time.Time                 `json:"receivedAt"`
}

/*
StateEntry is one persisted historical EnvelopeState: the envelope it belongs
to and the exact flatbuffer bytes the running binary produced at that boundary.
*/
type StateEntry struct {
	Envelope hindsight.EnvelopeRef `json:"envelope"`
	Payload  []byte                `json:"payload"`
}

/*
ListRuns returns every captured Run, newest first, as the actual Run records
persisted to the runs table.
*/
func (store *SQLite) ListRuns() ([]hindsight.Run, error) {
	if store == nil || store.database == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	rows, err := store.database.Query(
		`SELECT id, started_at, code_commit, build_id, config_digest,
		        schema_versions, integrity
		 FROM runs
		 ORDER BY started_at DESC`,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"store: list runs failed",
			err,
		))
	}

	defer rows.Close()

	runs := make([]hindsight.Run, 0)

	for rows.Next() {
		var (
			run          hindsight.Run
			startedAt    string
			versionsRaw  string
			integrityRaw string
		)

		if err := rows.Scan(
			&run.ID,
			&startedAt,
			&run.CodeCommit,
			&run.BuildID,
			&run.ConfigDigest,
			&versionsRaw,
			&integrityRaw,
		); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"store: scan run row",
				err,
			))
		}

		run.StartedAt, _ = time.Parse(time.RFC3339Nano, startedAt)
		run.Integrity = parseIntegrity(integrityRaw)
		run.SchemaVersions = decodeSchemaVersions(versionsRaw)

		runs = append(runs, run)
	}

	return runs, nil
}

/*
ListCaptures returns the raw captured frames of one Run in capture-sequence
order — the replay order (§52). It returns the exact CaptureIdentity recorded
for each frame, not a recomputed identity.
*/
func (store *SQLite) ListCaptures(runID string, limit int) ([]CaptureEntry, error) {
	if store == nil || store.database == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if runID == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: run identity required",
			nil,
		))
	}

	if limit <= 0 {
		limit = 500
	}

	rows, err := store.database.Query(
		`SELECT run_id, capture_seq, stream, stream_epoch, stream_seq,
		        kind, endpoint, at
		 FROM events
		 WHERE run_id = ?
		 ORDER BY capture_seq ASC
		 LIMIT ?`,
		runID,
		limit,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"store: list captures failed",
			err,
		))
	}

	defer rows.Close()

	captures := make([]CaptureEntry, 0)

	for rows.Next() {
		var (
			entry      CaptureEntry
			receivedAt string
		)

		if err := rows.Scan(
			&entry.Identity.Run,
			&entry.Identity.Sequence,
			&entry.Identity.Stream,
			&entry.Identity.StreamEpoch,
			&entry.Identity.StreamSequence,
			&entry.Kind,
			&entry.Endpoint,
			&receivedAt,
		); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"store: scan capture row",
				err,
			))
		}

		entry.ReceivedAt, _ = time.Parse(time.RFC3339Nano, receivedAt)

		captures = append(captures, entry)
	}

	return captures, nil
}

/*
ListStates returns every persisted historical EnvelopeState of one Run, in
capture order, each carrying its EnvelopeRef and the exact flatbuffer bytes.
*/
func (store *SQLite) ListStates(runID string) ([]StateEntry, error) {
	if store == nil || store.database == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if runID == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: run identity required",
			nil,
		))
	}

	rows, err := store.database.Query(
		`SELECT witness FROM witnesses
		 WHERE origin_run = ? AND artifact_kind = 'state'
		 ORDER BY origin_seq ASC`,
		runID,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"store: list states failed",
			err,
		))
	}

	defer rows.Close()

	states := make([]StateEntry, 0)

	for rows.Next() {
		var encoded string

		if err := rows.Scan(&encoded); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"store: scan state row",
				err,
			))
		}

		var witness hindsight.ArtifactWitness

		if err := decodeWitness(encoded, &witness); err != nil {
			return nil, err
		}

		states = append(states, StateEntry{
			Envelope: witness.Envelope,
			Payload:  witness.Payload,
		})
	}

	return states, nil
}

/*
parseIntegrity recovers the stored integrity string into the typed enum. An
unknown value fails closed to CORRUPT rather than silently reading COMPLETE.
*/
func parseIntegrity(raw string) hindsight.Integrity {
	switch raw {
	case "COMPLETE":
		return hindsight.IntegrityComplete
	case "GAPPED":
		return hindsight.IntegrityGapped
	case "CORRUPT":
		return hindsight.IntegrityCorrupt
	default:
		return hindsight.IntegrityCorrupt
	}
}

/*
decodeSchemaVersions recovers the run's schema-versions JSON map. A blank field
decodes to nil, not a fabricated map.
*/
func decodeSchemaVersions(raw string) map[string]string {
	if raw == "" {
		return nil
	}

	var versions map[string]string

	if err := json.Unmarshal([]byte(raw), &versions); err != nil {
		return nil
	}

	return versions
}

/*
decodeWitness recovers a persisted ArtifactWitness JSON blob. A malformed blob
is a loud error — never a silently empty witness.
*/
func decodeWitness(encoded string, witness *hindsight.ArtifactWitness) error {
	if err := json.Unmarshal([]byte(encoded), witness); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: decode witness failed",
			err,
		))
	}

	return nil
}
