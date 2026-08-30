package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
)

const eventSchema = `
CREATE TABLE IF NOT EXISTS events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    kind          TEXT    NOT NULL,
    endpoint      TEXT    NOT NULL DEFAULT '',
    at            TEXT    NOT NULL,
    data          BLOB    NOT NULL,
    run_id        TEXT    NOT NULL DEFAULT '',
    capture_seq   INTEGER NOT NULL DEFAULT 0,
    stream        TEXT    NOT NULL DEFAULT '',
    stream_epoch  INTEGER NOT NULL DEFAULT 0,
    stream_seq    INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX IF NOT EXISTS idx_events_kind ON events(kind);
CREATE INDEX IF NOT EXISTS idx_events_at   ON events(at);
`

const runSchema = `
CREATE TABLE IF NOT EXISTS runs (
    id              TEXT PRIMARY KEY,
    started_at      TEXT    NOT NULL,
    code_commit     TEXT    NOT NULL DEFAULT '',
    build_id        TEXT    NOT NULL DEFAULT '',
    config_digest   TEXT    NOT NULL DEFAULT '',
    schema_versions TEXT    NOT NULL DEFAULT '',
    integrity       TEXT    NOT NULL DEFAULT 'COMPLETE'
) STRICT;
`

const envelopeSchema = `
CREATE TABLE IF NOT EXISTS envelopes (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    envelope_ref   TEXT    NOT NULL,
    origin_run     TEXT    NOT NULL DEFAULT '',
    origin_seq     INTEGER NOT NULL DEFAULT 0,
    ordinal        INTEGER NOT NULL DEFAULT 0,
    workload       TEXT    NOT NULL DEFAULT '',
    domain_kind    TEXT    NOT NULL DEFAULT '',
    symbol         TEXT    NOT NULL DEFAULT '',
    manifest       TEXT    NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_envelopes_origin ON envelopes(origin_run, origin_seq);
`

const witnessSchema = `
CREATE TABLE IF NOT EXISTS witnesses (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    envelope_ref   TEXT    NOT NULL,
    origin_run     TEXT    NOT NULL DEFAULT '',
    origin_seq     INTEGER NOT NULL DEFAULT 0,
    artifact_kind  TEXT    NOT NULL DEFAULT '',
    artifact_id    TEXT    NOT NULL DEFAULT '',
    boundary       TEXT    NOT NULL DEFAULT '',
    witness        TEXT    NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_witnesses_origin ON witnesses(origin_run, origin_seq);
`

const gapSchema = `
CREATE TABLE IF NOT EXISTS gaps (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id     TEXT    NOT NULL,
    encoding   TEXT    NOT NULL,
    sequence   INTEGER NOT NULL DEFAULT 0,
    detail     TEXT    NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX IF NOT EXISTS idx_gaps_run ON gaps(run_id);
`

const lifecycleSchema = `
CREATE TABLE IF NOT EXISTS lifecycle (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id      TEXT    NOT NULL DEFAULT '',
    decision_id TEXT    NOT NULL DEFAULT '',
    symbol      TEXT    NOT NULL DEFAULT '',
    kind        TEXT    NOT NULL,
    action      TEXT    NOT NULL DEFAULT '',
    at          TEXT    NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_lifecycle_run ON lifecycle(run_id);
CREATE INDEX IF NOT EXISTS idx_lifecycle_decision ON lifecycle(decision_id);
`

const endpointIndex = `
CREATE INDEX IF NOT EXISTS idx_events_endpoint ON events(endpoint);
`

const endpointMigration = `
ALTER TABLE events ADD COLUMN endpoint TEXT NOT NULL DEFAULT '';
`

const identityMigration = `
ALTER TABLE events ADD COLUMN run_id TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN capture_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN stream TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN stream_epoch INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN stream_seq INTEGER NOT NULL DEFAULT 0;
`

/*
SQLite is the default Repository engine. It persists every WriteEvent as one
row in a single kind-tagged event table, so replay is a single ordered scan per
kind rather than a per-domain table sprawl. The endpoint column names the origin
stream (the websocket URL for raw frames, the layer name for stage snapshots)
without being welded onto the payload, so the data column stays verbatim and
each frame is addressable by its source. The connection is opened once and
serialized (one writer) with WAL and a busy timeout so the writer never blocks
readers and never loses a record to a transient lock.
*/
type SQLite struct {
	database *sql.DB
}

/*
NewSQLite opens (creating if absent) the SQLite database backing the store. An
empty path is a validation error; the engine owns schema creation on first use.
*/
func NewSQLite(path string) (*SQLite, error) {
	if path == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite path required",
			nil,
		))
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: mkdir failed for %s [%s]", filepath.Dir(path), err.Error()),
			err,
		))
	}

	database, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: open failed for %s [%s]", path, err.Error()),
			err,
		))
	}

	database.SetMaxOpenConns(1)

	store := &SQLite{database: database}

	if err := store.EnsureSchema(); err != nil {
		_ = database.Close()
		return nil, err
	}

	return store, nil
}

/*
EnsureSchema creates the event table and its indexes if they do not already
exist, and migrates pre-endpoint databases to carry the endpoint column. It is
safe to call repeatedly.
*/
func (store *SQLite) EnsureSchema() error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if _, err := store.database.Exec(eventSchema); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: ensure schema failed",
			err,
		))
	}

	if _, err := store.database.Exec(runSchema); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: ensure run schema failed",
			err,
		))
	}

	if _, err := store.database.Exec(envelopeSchema); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: ensure envelope schema failed",
			err,
		))
	}

	if _, err := store.database.Exec(witnessSchema); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: ensure witness schema failed",
			err,
		))
	}

	if _, err := store.database.Exec(gapSchema); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: ensure gap schema failed",
			err,
		))
	}

	if _, err := store.database.Exec(lifecycleSchema); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: ensure lifecycle schema failed",
			err,
		))
	}

	if err := store.migrateEndpoint(); err != nil {
		return err
	}

	if err := store.migrateIdentity(); err != nil {
		return err
	}

	// The endpoint index depends on the column that the migration above may
	// just have added, so it is created only after the column is guaranteed.
	if _, err := store.database.Exec(endpointIndex); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: ensure endpoint index failed",
			err,
		))
	}

	return nil
}

/*
migrateEndpoint adds the endpoint column to databases created before the
endpoint/payload split. The CREATE TABLE IF NOT EXISTS above leaves an existing
table untouched, so a pre-split database is upgraded here instead.
*/
func (store *SQLite) migrateEndpoint() error {
	if store.hasColumn("events", "endpoint") {
		return nil
	}

	if _, err := store.database.Exec(endpointMigration); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: add endpoint column to events",
			err,
		))
	}

	return nil
}

/*
migrateIdentity adds the Hindsight capture-identity columns to databases created
before capture provenance existed. Pre-existing rows default to empty identity
fields, marking them untraceable rather than fabricating an identity.
*/
func (store *SQLite) migrateIdentity() error {
	if store.hasColumn("events", "run_id") {
		return nil
	}

	if _, err := store.database.Exec(identityMigration); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: add capture identity columns to events",
			err,
		))
	}

	return nil
}

/*
hasColumn reports whether a table already carries a column. It drives the
idempotent migrations so a database is upgraded once, exactly, without risking
a duplicate-column error on a second open.
*/
func (store *SQLite) hasColumn(table, column string) bool {
	columns, err := store.database.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))

	if err != nil {
		return false
	}

	defer columns.Close()

	for columns.Next() {
		var (
			colID      int
			colName    string
			colType    string
			colNotNull int
			colDefault sql.NullString
			colPK      int
		)

		if err := columns.Scan(
			&colID, &colName, &colType, &colNotNull, &colDefault, &colPK,
		); err != nil {
			return false
		}

		if colName == column {
			return true
		}
	}

	return false
}

/*
WriteRun persists one process capture session's identity and metadata (§5).
*/
func (store *SQLite) WriteRun(run hindsight.Run) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if run.ID == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: run identity required",
			nil,
		))
	}

	schemaVersions := ""

	if run.SchemaVersions != nil {
		encoded, err := json.Marshal(run.SchemaVersions)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"store: encode run schema versions",
				err,
			))
		}

		schemaVersions = string(encoded)
	}

	if _, err := store.database.Exec(
		`INSERT OR REPLACE INTO runs
		 (id, started_at, code_commit, build_id, config_digest, schema_versions, integrity)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(run.ID),
		run.StartedAt.UTC().Format(time.RFC3339Nano),
		run.CodeCommit,
		run.BuildID,
		run.ConfigDigest,
		schemaVersions,
		run.Integrity.String(),
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: write run failed [%s]", err.Error()),
			err,
		))
	}

	return nil
}

/*
WriteCapture persists one raw transport frame together with the stable
CaptureIdentity assigned to it before parsing. The identity columns are stored
alongside the payload so raw capture and semantic ingress are joinable by
identity, never by timestamp.
*/
func (store *SQLite) WriteCapture(
	identity hindsight.CaptureIdentity,
	endpoint, kind string,
	payload []byte,
	at time.Time,
) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if !identity.Valid() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: capture identity required",
			nil,
		))
	}

	if _, err := store.database.Exec(
		`INSERT INTO events
		 (kind, endpoint, at, data, run_id, capture_seq, stream, stream_epoch, stream_seq)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		kind,
		endpoint,
		at.UTC().Format(time.RFC3339Nano),
		payload,
		string(identity.Run),
		uint64(identity.Sequence),
		string(identity.Stream),
		uint64(identity.StreamEpoch),
		identity.StreamSequence,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: write %s capture failed [%s]", kind, err.Error()),
			err,
		))
	}

	return nil
}

/*
WriteFrame persists one raw transport frame without a Hindsight identity. The
payload is stored verbatim and tagged with its endpoint and kind; the identity
columns stay empty, marking the frame as untraceable rather than silently
claiming provenance.
*/
func (store *SQLite) WriteFrame(endpoint, kind string, payload []byte, at time.Time) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if _, err := store.database.Exec(
		"INSERT INTO events (kind, endpoint, at, data) VALUES (?, ?, ?, ?)",
		kind,
		endpoint,
		at.UTC().Format(time.RFC3339Nano),
		payload,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: write %s event failed [%s]", kind, err.Error()),
			err,
		))
	}

	return nil
}

/*
WriteManifest persists one EnvelopeManifest — how one raw frame entered
Workspace — keyed by its EnvelopeRef. The origin run/sequence/ordinal are stored
as columns so the raw-frame → envelope fan-out is joinable by identity, never
by timestamp.
*/
func (store *SQLite) WriteManifest(manifest hindsight.EnvelopeManifest) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if !manifest.Envelope.Origin.Valid() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: envelope manifest requires a valid origin",
			nil,
		))
	}

	ref, err := hindsight.MarshalEnvelopeRef(manifest.Envelope)

	if err != nil {
		return err
	}

	payload, err := hindsight.MarshalManifest(manifest)

	if err != nil {
		return err
	}

	if _, err := store.database.Exec(
		`INSERT INTO envelopes
		 (envelope_ref, origin_run, origin_seq, ordinal, workload, domain_kind, symbol, manifest)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ref,
		string(manifest.Envelope.Origin.Run),
		uint64(manifest.Envelope.Origin.Sequence),
		manifest.Envelope.Ordinal,
		manifest.Workload,
		manifest.DomainKind,
		manifest.Symbol,
		payload,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: write envelope manifest failed [%s]", err.Error()),
			err,
		))
	}

	return nil
}

/*
WriteWitness persists one ArtifactWitness — the semantic artifact the running
binary actually produced at a Workspace boundary, with its exact parent and
resident-state provenance.
*/
func (store *SQLite) WriteWitness(witness hindsight.ArtifactWitness) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if !witness.Envelope.Origin.Valid() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: artifact witness requires a valid origin",
			nil,
		))
	}

	ref, err := hindsight.MarshalEnvelopeRef(witness.Envelope)

	if err != nil {
		return err
	}

	payload, err := hindsight.MarshalWitness(witness)

	if err != nil {
		return err
	}

	if _, err := store.database.Exec(
		`INSERT INTO witnesses
		 (envelope_ref, origin_run, origin_seq, artifact_kind, artifact_id, boundary, witness)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ref,
		string(witness.Envelope.Origin.Run),
		uint64(witness.Envelope.Origin.Sequence),
		witness.Artifact.Kind,
		witness.Artifact.Identity,
		witness.Boundary,
		payload,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: write artifact witness failed [%s]", err.Error()),
			err,
		))
	}

	return nil
}

/*
MarkGapped persists one concrete capture defect and flips the run's integrity
to GAPPED. It never demotes a run that is already CORRUPT: the highest-severity
verdict wins. detail carries the exact failure message so an inspector can read
why the run is not complete.
*/
func (store *SQLite) MarkGapped(
	runID hindsight.RunID,
	sequence hindsight.CaptureSequence,
	encoding string,
	detail string,
) error {
	return store.recordGap(runID, sequence, encoding, detail, "GAPPED")
}

/*
MarkCorrupt persists one concrete integrity/provenance defect and flips the run
to CORRUPT. Corruption is the strongest verdict: it always overrides COMPLETE or
GAPPED.
*/
func (store *SQLite) MarkCorrupt(
	runID hindsight.RunID,
	sequence hindsight.CaptureSequence,
	encoding string,
	detail string,
) error {
	return store.recordGap(runID, sequence, encoding, detail, "CORRUPT")
}

/*
recordGap is the shared write path for a concrete defect: persist the Gap row,
then raise the run's integrity to the given severity. GAPPED is not applied over
an existing CORRUPT so the stronger verdict survives; CORRUPT always applies.
*/
func (store *SQLite) recordGap(
	runID hindsight.RunID,
	sequence hindsight.CaptureSequence,
	encoding string,
	detail string,
	severity string,
) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if runID == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: run identity required",
			nil,
		))
	}

	if _, err := store.database.Exec(
		"INSERT INTO gaps (run_id, encoding, sequence, detail) VALUES (?, ?, ?, ?)",
		string(runID),
		encoding,
		uint64(sequence),
		detail,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: record gap failed",
			err,
		))
	}

	if severity == "CORRUPT" {
		if _, err := store.database.Exec(
			"UPDATE runs SET integrity = 'CORRUPT' WHERE id = ?",
			string(runID),
		); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"store: mark run corrupt failed",
				err,
			))
		}

		return nil
	}

	if _, err := store.database.Exec(
		`UPDATE runs SET integrity = 'GAPPED'
		 WHERE id = ? AND integrity != 'CORRUPT'`,
		string(runID),
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: mark run gapped failed",
			err,
		))
	}

	return nil
}

/*
WriteLifecycleEvent persists one real trading-lifecycle transition, correlated
by the decision ID that caused it and tagged with the run. This is observational
recording — a failure here must never affect trading progress.
*/
func (store *SQLite) WriteLifecycleEvent(
	runID hindsight.RunID,
	event hindsight.LifecycleEvent,
) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if event.DecisionID == "" || event.Kind == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: lifecycle decision id and kind required",
			nil,
		))
	}

	if _, err := store.database.Exec(
		`INSERT INTO lifecycle (run_id, decision_id, symbol, kind, action, at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		string(runID),
		event.DecisionID,
		event.Symbol,
		event.Kind,
		event.Action,
		event.At.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: write lifecycle event failed",
			err,
		))
	}

	return nil
}

/*
ReadCapture returns the payload bytes of the raw frame persisted under exactly
one CaptureIdentity. It is an exact-identity read — no timestamp search — and is
the durable reverse of WriteCapture: what the process actually received.
*/
func (store *SQLite) ReadCapture(identity hindsight.CaptureIdentity) ([]byte, error) {
	if store == nil || store.database == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if !identity.Valid() {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: capture identity required",
			nil,
		))
	}

	var payload []byte

	err := store.database.QueryRow(
		"SELECT data FROM events WHERE run_id = ? AND capture_seq = ?",
		string(identity.Run),
		uint64(identity.Sequence),
	).Scan(&payload)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"store: read capture failed",
			err,
		))
	}

	return payload, nil
}

/*
ReadWitness returns the witness record for one ArtifactID persisted under one
origin, reconstructing the artifact → envelope → raw-frame chain by identity.
*/
func (store *SQLite) ReadWitness(origin hindsight.CaptureIdentity, artifact string) (hindsight.ArtifactWitness, error) {
	if store == nil || store.database == nil {
		return hindsight.ArtifactWitness{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	var encoded string

	err := store.database.QueryRow(
		"SELECT witness FROM witnesses WHERE origin_run = ? AND origin_seq = ? AND artifact_id = ?",
		string(origin.Run),
		uint64(origin.Sequence),
		artifact,
	).Scan(&encoded)

	if err != nil {
		return hindsight.ArtifactWitness{}, errnie.Error(errnie.Err(
			errnie.IO,
			"store: read witness failed",
			err,
		))
	}

	var witness hindsight.ArtifactWitness

	if err := json.Unmarshal([]byte(encoded), &witness); err != nil {
		return hindsight.ArtifactWitness{}, errnie.Error(errnie.Err(
			errnie.IO,
			"store: decode witness failed",
			err,
		))
	}

	return witness, nil
}

/*
Close releases the database handle.
*/
func (store *SQLite) Close() error {
	if store == nil || store.database == nil {
		return nil
	}

	return store.database.Close()
}
