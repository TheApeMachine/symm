package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/klauspost/compress/zstd"
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
    stream_seq    INTEGER NOT NULL DEFAULT 0,
    encoding      TEXT    NOT NULL DEFAULT 'identity'
) STRICT;
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
    at          TEXT    NOT NULL,
    execution   TEXT    NOT NULL DEFAULT '',
    capture_seq INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX IF NOT EXISTS idx_lifecycle_run ON lifecycle(run_id);
CREATE INDEX IF NOT EXISTS idx_lifecycle_decision ON lifecycle(decision_id);
`

const lifecycleExecutionMigration = `
ALTER TABLE lifecycle ADD COLUMN execution TEXT NOT NULL DEFAULT '';
`

const lifecycleCaptureSeqMigration = `
ALTER TABLE lifecycle ADD COLUMN capture_seq INTEGER NOT NULL DEFAULT 0;
`

const endpointMigration = `
ALTER TABLE events ADD COLUMN endpoint TEXT NOT NULL DEFAULT '';
`

const eventEncodingMigration = `
ALTER TABLE events ADD COLUMN encoding TEXT NOT NULL DEFAULT 'identity';
`

const obsoleteEventIndexes = `
DROP INDEX IF EXISTS idx_events_kind;
DROP INDEX IF EXISTS idx_events_at;
DROP INDEX IF EXISTS idx_events_endpoint;
DROP INDEX IF EXISTS idx_events_run_kind;
`

const identityMigration = `
ALTER TABLE events ADD COLUMN run_id TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN capture_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN stream TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN stream_epoch INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN stream_seq INTEGER NOT NULL DEFAULT 0;
`

/*
maxReaderConns bounds the inspection pool. Inspection is a single operator
looking at one frame at a time, and the surface issues a handful of reads per
parked playhead, so a small pool serves it without letting a browser open an
unbounded number of scans against the capture tape.
*/
const maxReaderConns = 4

/*
SQLite is the default Repository engine. It persists every WriteEvent as one
row in a single kind-tagged event table, so replay is a single ordered scan per
kind rather than a per-domain table sprawl. The endpoint column names the origin
stream. Payload bytes use zstd only when it is smaller than identity storage;
every repository read reverses that encoding and returns the exact input bytes.
The connection is opened once and serialized with WAL and a busy timeout.
*/
type SQLite struct {
	database *sql.DB
	/*
		reader is the inspection path's own connection pool.

		The write connection is deliberately serialised to one, and the capture
		writer commits raw market input over it. An inspection read sharing that
		connection therefore queues ahead of recording the market: a long read
		holds the connection, the writer's bounded queue fills behind it, and
		capture — which is not replayable — stalls waiting on a browser.

		WAL admits readers concurrently with the writer, so inspection reads run
		here instead. They are opened read-only, which makes the separation a
		property the database enforces rather than a convention this package has
		to remember.
	*/
	reader  *sql.DB
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

/*
Reader returns the connection inspection reads must use. It is never the
capture writer's connection, so no inspection query can delay recording the
market.
*/
func (store *SQLite) Reader() *sql.DB {
	if store == nil || store.reader == nil {
		return nil
	}

	return store.reader
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

	database, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: open failed for %s [%s]", path, err.Error()),
			err,
		))
	}

	database.SetMaxOpenConns(1)

	// Inspection reads get their own read-only pool. WAL lets these proceed
	// concurrently with the writer, so a slow read can no longer hold the
	// connection the capture path commits over.
	// The file: prefix is required: without it go-sqlite3 treats the string as
	// a bare path and silently ignores mode=ro, which would leave the
	// inspection pool writable and the separation unenforced.
	reader, err := sql.Open(
		"sqlite3",
		"file:"+path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&mode=ro",
	)

	if err != nil {
		_ = database.Close()

		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: open reader failed for %s [%s]", path, err.Error()),
			err,
		))
	}

	reader.SetMaxOpenConns(maxReaderConns)

	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderConcurrency(1),
	)

	if err != nil {
		_ = reader.Close()
		_ = database.Close()
		return nil, fmt.Errorf("store: construct zstd encoder: %w", err)
	}

	decoder, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))

	if err != nil {
		encoder.Close()
		_ = reader.Close()
		_ = database.Close()
		return nil, fmt.Errorf("store: construct zstd decoder: %w", err)
	}

	store := &SQLite{
		database: database,
		reader:   reader,
		encoder:  encoder,
		decoder:  decoder,
	}

	if err := store.EnsureSchema(); err != nil {
		_ = store.Close()
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

	if err := store.migrateLifecycleCaptureSeq(); err != nil {
		return err
	}

	if err := store.migrateLifecycleExecution(); err != nil {
		return err
	}

	if err := store.migrateEndpoint(); err != nil {
		return err
	}

	if err := store.migrateIdentity(); err != nil {
		return err
	}

	// The learning journal shares this database and writer, with its own small
	// index. Creating a partial index on raw captures would scan the entire tape.
	if _, err := store.database.Exec(learningSchema); err != nil {
		return errnie.Err(errnie.IO, "store: ensure learning journal", err)
	}

	if err := store.migrateEventEncoding(); err != nil {
		return err
	}

	// These historical indexes are not used by any repository read. Maintaining
	// one index per timestamp, kind, and endpoint multiplied the write volume of
	// the raw tape without making an actual query cheaper. The run/kind/capture
	// index below is the one Episode discovery uses.
	if _, err := store.database.Exec(obsoleteEventIndexes); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: remove unused event indexes failed",
			err,
		))
	}

	// The capture-market index likewise depends on the identity columns the
	// migration may just have added. It serves the per-run market read that
	// Episode discovery walks, in capture order.
	if _, err := store.database.Exec(captureMarketIndex); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: ensure capture market index failed",
			err,
		))
	}

	// Identity-addressed reads constrain (run_id, capture_seq) without naming a
	// kind, so they cannot use the partial index above and would otherwise scan
	// the entire capture tape.
	if _, err := store.database.Exec(captureIdentityIndex); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: ensure capture identity index failed",
			err,
		))
	}

	return nil
}

func (store *SQLite) migrateEventEncoding() error {
	if store.hasColumn("events", "encoding") {
		return nil
	}

	if _, err := store.database.Exec(eventEncodingMigration); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: add event encoding column",
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
migrateLifecycleExecution adds the execution column to lifecycle databases
created before fill facts were recorded. Pre-existing rows default to an empty
execution payload, marking them as transition-only events.
*/
func (store *SQLite) migrateLifecycleCaptureSeq() error {
	if store.hasColumn("lifecycle", "capture_seq") {
		return nil
	}

	if _, err := store.database.Exec(lifecycleCaptureSeqMigration); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: add capture_seq column to lifecycle",
			err,
		))
	}

	return nil
}

/*
migrateLifecycleExecution adds the venue execution column to a lifecycle table
written before it existed.
*/
func (store *SQLite) migrateLifecycleExecution() error {
	if store.hasColumn("lifecycle", "execution") {
		return nil
	}

	if _, err := store.database.Exec(lifecycleExecutionMigration); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: add execution column to lifecycle",
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
	if store == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	return store.writeOperation(store.database, writerOperation{
		kind:        writerCapture,
		identity:    identity,
		endpoint:    endpoint,
		captureKind: kind,
		payload:     payload,
		at:          at,
	})
}

/*
WriteFrame persists one raw transport frame without a Hindsight identity. The
payload remains byte-exact through the row's reversible encoding and is tagged
with its endpoint and kind; the identity columns stay empty, marking the frame
as untraceable rather than silently claiming provenance.
*/
func (store *SQLite) WriteFrame(endpoint, kind string, payload []byte, at time.Time) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	storedPayload, encoding := store.encodePayload(payload)

	if _, err := store.database.Exec(
		"INSERT INTO events (kind, endpoint, at, data, encoding) VALUES (?, ?, ?, ?, ?)",
		kind,
		endpoint,
		at.UTC().Format(time.RFC3339Nano),
		storedPayload,
		encoding,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: write %s event failed [%s]", kind, err.Error()),
			err,
		))
	}

	return nil
}

func (store *SQLite) encodePayload(payload []byte) ([]byte, string) {
	if len(payload) < 1024 {
		return payload, "identity"
	}

	compressed := store.encoder.EncodeAll(payload, nil)

	if len(compressed) >= len(payload) {
		return payload, "identity"
	}

	return compressed, "zstd"
}

func (store *SQLite) decodePayload(payload []byte, encoding string) ([]byte, error) {
	switch encoding {
	case "identity", "":
		return payload, nil
	case "zstd":
		decoded, err := store.decoder.DecodeAll(payload, nil)

		if err != nil {
			return nil, fmt.Errorf("store: decode zstd capture: %w", err)
		}

		return decoded, nil
	default:
		return nil, fmt.Errorf("store: unsupported capture encoding %q", encoding)
	}
}

/*
WriteManifest persists one EnvelopeManifest — how one raw frame entered
Workspace — keyed by its EnvelopeRef. The origin run/sequence/ordinal are stored
as columns so the raw-frame → envelope fan-out is joinable by identity, never
by timestamp.
*/
func (store *SQLite) WriteManifest(manifest hindsight.EnvelopeManifest) error {
	if store == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	return store.writeOperation(store.database, writerOperation{
		kind:     writerManifest,
		manifest: manifest,
	})
}

/*
WriteWitness persists one ArtifactWitness — the semantic artifact the running
binary actually produced at a Workspace boundary, with its exact parent and
resident-state provenance. The marshal and insert live here so a single-writer
caller gets one durable witness per call.
*/
func (store *SQLite) WriteWitness(witness hindsight.ArtifactWitness) error {
	return store.WriteWitnesses([]hindsight.ArtifactWitness{witness})
}

/*
WriteWitnesses persists a batch of ArtifactWitness records in one transaction.
Batching is the throughput-preserving persistence shape: a background worker
accumulates witnesses off the hot path and commits them together instead of
locking the database with one INSERT per frame. It is not part of the
Repository interface; repositories that implement it opt into batched writes,
and the async writer falls back to WriteWitness otherwise.
*/
func (store *SQLite) WriteWitnesses(witnesses []hindsight.ArtifactWitness) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if len(witnesses) == 0 {
		return nil
	}

	transaction, err := store.database.Begin()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: begin witness batch transaction failed",
			err,
		))
	}

	// Rollback is a no-op after a successful Commit; the deferred call only
	// runs when the loop returns an error, which is the intended semantics.
	defer func() { _ = transaction.Rollback() }()

	for _, witness := range witnesses {
		if !witness.Envelope.Origin.Valid() {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"store: artifact witness requires a valid origin",
				nil,
			))
		}

		ref, marshalErr := hindsight.MarshalEnvelopeRef(witness.Envelope)

		if marshalErr != nil {
			return marshalErr
		}

		payload, marshalErr := hindsight.MarshalWitness(witness)

		if marshalErr != nil {
			return marshalErr
		}

		if _, execErr := transaction.Exec(
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
		); execErr != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				fmt.Sprintf("store: write artifact witness failed [%s]", execErr.Error()),
				execErr,
			))
		}
	}

	if err := transaction.Commit(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: commit witness batch transaction failed",
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
	if store == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	return store.writeLifecycleOperation(store.database, runID, event)
}

/*
ReadCapture returns the payload bytes of the raw frame persisted under exactly
one CaptureIdentity. It is an exact-identity read — no timestamp search — and is
the durable reverse of WriteCapture: what the process actually received.
*/
func (store *SQLite) ReadCapture(identity hindsight.CaptureIdentity) ([]byte, error) {
	if store == nil || store.reader == nil {
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

	var (
		payload  []byte
		encoding string
	)

	err := store.reader.QueryRow(
		"SELECT data, encoding FROM events WHERE run_id = ? AND capture_seq = ?",
		string(identity.Run),
		uint64(identity.Sequence),
	).Scan(&payload, &encoding)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"store: read capture failed",
			err,
		))
	}

	decoded, err := store.decodePayload(payload, encoding)

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.IO, "store: decode capture failed", err))
	}

	return decoded, nil
}

/*
ReadWitness returns the witness record for one ArtifactID persisted under one
origin, reconstructing the artifact → envelope → raw-frame chain by identity.
*/
func (store *SQLite) ReadWitness(origin hindsight.CaptureIdentity, artifact string) (hindsight.ArtifactWitness, error) {
	if store == nil || store.reader == nil {
		return hindsight.ArtifactWitness{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	var encoded string

	err := store.reader.QueryRow(
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

	if store.encoder != nil {
		store.encoder.Close()
	}

	if store.decoder != nil {
		store.decoder.Close()
	}

	if store.reader != nil {
		_ = store.reader.Close()
	}

	return store.database.Close()
}
