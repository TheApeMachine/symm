package store

import (
	"database/sql"
	"encoding/json"
	"errors"
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
ReadStatePayload returns the persisted EnvelopeState bytes for one exact
EnvelopeRef, and whether one was witnessed there at all. It is the narrow read
the as-of resolution walks backwards with: a point read by identity, never a
scan and never a search.
*/
func (store *SQLite) ReadStatePayload(
	runID string,
	sequence uint64,
	ordinal uint64,
) ([]byte, bool, error) {
	entry, found, err := store.ReadState(runID, sequence, ordinal)

	if err != nil || !found {
		return nil, false, err
	}

	return entry.Payload, true, nil
}

/*
ReadCaptureFrame returns the raw frame at one capture sequence of one Run,
together with the full CaptureIdentity the process assigned it.

CaptureSequence is monotonic within a Run (§6), so (run, sequence) already
names exactly one external input. This is the read for a caller holding that
run-local coordinate — a UI addressing a frame it has on screen — and it
answers with the complete identity rather than making the caller reconstruct
the transport fields it did not have.
*/
func (store *SQLite) ReadCaptureFrame(
	runID string,
	sequence uint64,
) (CaptureEntry, []byte, bool, error) {
	if store == nil || store.database == nil {
		return CaptureEntry{}, nil, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if runID == "" {
		return CaptureEntry{}, nil, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: run identity required",
			nil,
		))
	}

	var (
		entry      CaptureEntry
		receivedAt string
		payload    []byte
	)

	entry.Identity.Run = hindsight.RunID(runID)

	err := store.database.QueryRow(
		`SELECT capture_seq, stream, stream_epoch, stream_seq, kind, endpoint, at, data
		 FROM events
		 WHERE run_id = ? AND capture_seq = ?`,
		runID,
		sequence,
	).Scan(
		&entry.Identity.Sequence,
		&entry.Identity.Stream,
		&entry.Identity.StreamEpoch,
		&entry.Identity.StreamSequence,
		&entry.Kind,
		&entry.Endpoint,
		&receivedAt,
		&payload,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return CaptureEntry{}, nil, false, nil
	}

	if err != nil {
		return CaptureEntry{}, nil, false, errnie.Error(errnie.Err(
			errnie.IO,
			"store: read capture frame failed",
			err,
		))
	}

	entry.ReceivedAt, _ = time.Parse(time.RFC3339Nano, receivedAt)

	return entry, payload, true, nil
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
GapView is one concrete capture/integrity defect the store recorded: its
encoding, the capture sequence it affects (0 when not pinned to a frame), and
the exact failure detail.
*/
type GapView struct {
	RunID    string `json:"runId"`
	Encoding string `json:"encoding"`
	Sequence uint64 `json:"sequence"`
	Detail   string `json:"detail"`
}

/*
ListGaps returns every recorded defect for one run, oldest first, so the UI can
show exactly why a run is GAPPED or CORRUPT.
*/
func (store *SQLite) ListGaps(runID string) ([]GapView, error) {
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
		"SELECT run_id, encoding, sequence, detail FROM gaps WHERE run_id = ? ORDER BY id ASC",
		runID,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"store: list gaps failed",
			err,
		))
	}

	defer rows.Close()

	gaps := make([]GapView, 0)

	for rows.Next() {
		var view GapView

		if err := rows.Scan(&view.RunID, &view.Encoding, &view.Sequence, &view.Detail); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"store: scan gap row",
				err,
			))
		}

		gaps = append(gaps, view)
	}

	return gaps, nil
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
ListCapturesAfter returns the raw captured frames with capture_seq strictly
greater than afterSeq, in capture order, bounded by limit. It is the paginated
timeline read: the UI walks the causal tape in fixed pages without ever loading
a whole run. The ordering is CaptureSequence — never venue/receive time.
*/
func (store *SQLite) ListCapturesAfter(runID string, afterSeq uint64, limit int) ([]CaptureEntry, error) {
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
		 WHERE run_id = ? AND capture_seq > ?
		 ORDER BY capture_seq ASC
		 LIMIT ?`,
		runID,
		afterSeq,
		limit,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"store: list captures after failed",
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
				"store: scan capture-after row",
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
ReadState returns the single persisted EnvelopeState for one exact EnvelopeRef
(run + capture sequence + ordinal), rather than every state of the run. It is
the exact-identity reverse of the witness node's "state" write.
*/
func (store *SQLite) ReadState(runID string, sequence uint64, ordinal uint64) (StateEntry, bool, error) {
	if store == nil || store.database == nil {
		return StateEntry{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if runID == "" {
		return StateEntry{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: run identity required",
			nil,
		))
	}

	rows, err := store.database.Query(
		`SELECT witness FROM witnesses
		 WHERE origin_run = ? AND origin_seq = ? AND artifact_kind = 'state'`,
		runID,
		sequence,
	)

	if err != nil {
		return StateEntry{}, false, errnie.Error(errnie.Err(
			errnie.IO,
			"store: read state failed",
			err,
		))
	}

	defer rows.Close()

	for rows.Next() {
		var encoded string

		if err := rows.Scan(&encoded); err != nil {
			return StateEntry{}, false, errnie.Error(errnie.Err(
				errnie.IO,
				"store: scan state row",
				err,
			))
		}

		var witness hindsight.ArtifactWitness

		if err := decodeWitness(encoded, &witness); err != nil {
			return StateEntry{}, false, err
		}

		if witness.Envelope.Ordinal != ordinal {
			continue
		}

		return StateEntry{
			Envelope: witness.Envelope,
			Payload:  witness.Payload,
		}, true, nil
	}

	return StateEntry{}, false, nil
}

/*
ListManifestsForCapture returns every EnvelopeManifest whose origin is one exact
capture sequence, in ordinal order — the raw-frame → envelope fan-out for a
single capture.
*/
func (store *SQLite) ListManifestsForCapture(runID string, sequence uint64) ([]hindsight.EnvelopeManifest, error) {
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
		`SELECT manifest FROM envelopes
		 WHERE origin_run = ? AND origin_seq = ?
		 ORDER BY ordinal ASC`,
		runID,
		sequence,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"store: list manifests for capture failed",
			err,
		))
	}

	defer rows.Close()

	manifests := make([]hindsight.EnvelopeManifest, 0)

	for rows.Next() {
		var encoded string

		if err := rows.Scan(&encoded); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"store: scan manifest row",
				err,
			))
		}

		var manifest hindsight.EnvelopeManifest

		if err := json.Unmarshal([]byte(encoded), &manifest); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"store: decode manifest failed",
				err,
			))
		}

		manifests = append(manifests, manifest)
	}

	return manifests, nil
}

/*
ListWitnessesForCapture returns every artifact witness whose origin is one exact
capture sequence, in ordinal then boundary order — what the running binary
produced from that raw frame.
*/
func (store *SQLite) ListWitnessesForCapture(runID string, sequence uint64) ([]hindsight.ArtifactWitness, error) {
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
		 WHERE origin_run = ? AND origin_seq = ?
		 ORDER BY id ASC`,
		runID,
		sequence,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"store: list witnesses for capture failed",
			err,
		))
	}

	defer rows.Close()

	witnesses := make([]hindsight.ArtifactWitness, 0)

	for rows.Next() {
		var encoded string

		if err := rows.Scan(&encoded); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"store: scan witness row",
				err,
			))
		}

		var witness hindsight.ArtifactWitness

		if err := decodeWitness(encoded, &witness); err != nil {
			return nil, err
		}

		witnesses = append(witnesses, witness)
	}

	return witnesses, nil
}

/*
ListLifecycleEvents returns every trading-lifecycle event for one run, oldest
first, each carrying its decision-ID correlation so the UI can join it to the
decision witness that caused it.
*/
func (store *SQLite) ListLifecycleEvents(runID string) ([]hindsight.LifecycleEvent, error) {
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
		`SELECT decision_id, symbol, kind, action, at, execution
		 FROM lifecycle WHERE run_id = ? ORDER BY id ASC`,
		runID,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"store: list lifecycle events failed",
			err,
		))
	}

	defer rows.Close()

	events := make([]hindsight.LifecycleEvent, 0)

	for rows.Next() {
		var (
			event     hindsight.LifecycleEvent
			atValue   string
			execution string
		)

		if err := rows.Scan(
			&event.DecisionID,
			&event.Symbol,
			&event.Kind,
			&event.Action,
			&atValue,
			&execution,
		); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"store: scan lifecycle event row",
				err,
			))
		}

		event.At, _ = time.Parse(time.RFC3339Nano, atValue)

		// The venue's fill facts are the authoritative execution record for
		// this transition. They were being written and then dropped on read,
		// which left every recorded fill invisible to inspection.
		if execution != "" {
			var fact hindsight.ExecutionFact

			if err := json.Unmarshal([]byte(execution), &fact); err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.IO,
					"store: decode lifecycle execution fact",
					err,
				))
			}

			event.Execution = &fact
		}

		events = append(events, event)
	}

	return events, nil
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
