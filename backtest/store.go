package backtest

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/theapemachine/errnie"
	"golang.design/x/lockfree/lf"
)

/*
Frame is one captured transport payload with its arrival time, exactly as the
venue delivered it. Payload bytes are stored verbatim so replay reproduces
the live byte stream, checksums included.
*/
type Frame struct {
	Endpoint   string
	Payload    []byte
	ReceivedAt time.Time
}

/*
CaptureInfo describes one recorded run.
*/
type CaptureInfo struct {
	ID        int64      `json:"id"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
	Frames    int64      `json:"frames"`
}

const captureSchema = `
CREATE TABLE IF NOT EXISTS captures (
	id INTEGER PRIMARY KEY,
	started_at TEXT NOT NULL,
	ended_at TEXT,
	frames INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE IF NOT EXISTS capture_frames (
	capture_id INTEGER NOT NULL,
	seq INTEGER NOT NULL,
	received_at TEXT NOT NULL,
	endpoint TEXT NOT NULL,
	payload BLOB NOT NULL,
	PRIMARY KEY (capture_id, seq)
) STRICT;

CREATE INDEX IF NOT EXISTS capture_frames_time
	ON capture_frames (capture_id, received_at);

CREATE TABLE IF NOT EXISTS audit_events (
	id INTEGER PRIMARY KEY,
	at TEXT NOT NULL,
	kind TEXT NOT NULL,
	payload BLOB NOT NULL
) STRICT;
`

/*
Store owns the capture tables inside the runtime sqlite database. Every live
run appends its raw transport frames here, and the backtest driver replays
them byte-for-byte.
*/
type Store struct {
	database *sql.DB
	path     string
}

/*
NewStore opens the capture store on an existing sqlite database file.
*/
func NewStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("backtest: store path required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("backtest: create store directory: %w", err)
	}

	database, err := sql.Open(
		"sqlite3",
		path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=-65536",
	)

	if err != nil {
		return nil, fmt.Errorf("backtest: open store: %w", err)
	}

	// WAL readers never block each other, so the pool can serve the streaming
	// playback cursor and the dashboard's capture listing at the same time.
	// A single connection used to starve the listing while a replay pumped.
	database.SetMaxOpenConns(maxStoreConnections)
	database.SetMaxIdleConns(maxStoreConnections)

	store := &Store{database: database, path: path}

	if err := store.EnsureSchema(); err != nil {
		database.Close()
		return nil, fmt.Errorf("backtest: migrate store: %w", err)
	}

	return store, nil
}

/*
EnsureSchema creates the required tables and indexes if they do not already exist.
*/
func (store *Store) EnsureSchema() error {
	if store == nil || store.database == nil {
		return fmt.Errorf("backtest: store database required")
	}

	if _, err := store.database.Exec(captureSchema); err != nil {
		return fmt.Errorf("backtest: migrate store: %w", err)
	}

	if err := migrateCaptureCounts(store.database); err != nil {
		return fmt.Errorf("backtest: migrate capture counts: %w", err)
	}

	return nil
}

const maxStoreConnections = 4

/*
migrateCaptureCounts brings pre-counter capture stores forward: it adds the
frames column when it is missing and backfills any zero-count rows that
already hold recorded frames. The backfill is self-healing — a genuinely
empty capture costs one indexed probe on the next boot and is skipped ever
after — so it never rehearses a fixed migration version.
*/
func migrateCaptureCounts(database *sql.DB) error {
	columns, err := database.Query("PRAGMA table_info(captures)")

	if err != nil {
		return err
	}

	hasFrames := false

	for columns.Next() {
		var cid, notNull, primaryKey int
		var name, columnType, defaultValue sql.NullString

		if err := columns.Scan(
			&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey,
		); err != nil {
			columns.Close()
			return err
		}

		if name.String == "frames" {
			hasFrames = true
		}
	}

	if err := columns.Close(); err != nil {
		return err
	}

	if !hasFrames {
		if _, err := database.Exec(
			"ALTER TABLE captures ADD COLUMN frames INTEGER NOT NULL DEFAULT 0",
		); err != nil {
			return err
		}
	}

	_, err = database.Exec(`
		UPDATE captures SET frames = (
			SELECT COUNT(*) FROM capture_frames f WHERE f.capture_id = captures.id
		) WHERE frames = 0
	`)

	return err
}

/*
Reopen returns an independent connection pool to the same store file. The
playback session holds one pooled connection for the whole stream, so an
analysis pass that also streams must not share that pool: SQLite WAL lets the
second reader scan concurrently instead of parking behind the playback query.
*/
func (store *Store) Reopen() (*Store, error) {
	if store == nil || store.path == "" {
		return nil, fmt.Errorf("backtest: reopen store: no path")
	}

	return NewStore(store.path)
}

/*
Close releases the database handle.
*/
func (store *Store) Close() error {
	if store == nil || store.database == nil {
		return nil
	}

	return store.database.Close()
}

/*
OpenCapture starts one run's recording and returns its frame sink. Frames are
buffered and committed in batches so the live write path stays cheap.
*/
func (store *Store) OpenCapture() (*CaptureWriter, error) {
	startedAt := time.Now().UTC()
	result, err := store.database.Exec(
		"INSERT INTO captures (started_at, frames) VALUES (?, 0)",
		startedAt.Format(time.RFC3339Nano),
	)

	if err != nil && isNoSuchTable(err) {
		if schemaErr := store.EnsureSchema(); schemaErr != nil {
			return nil, fmt.Errorf("backtest: ensure schema: %w", schemaErr)
		}

		result, err = store.database.Exec(
			"INSERT INTO captures (started_at, frames) VALUES (?, 0)",
			startedAt.Format(time.RFC3339Nano),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("backtest: open capture: %w", err)
	}

	id, err := result.LastInsertId()

	if err != nil {
		return nil, fmt.Errorf("backtest: capture id: %w", err)
	}

	return &CaptureWriter{
		store:     store,
		id:        id,
		startedAt: startedAt,
		committed: make([]Frame, 0, captureBatchSize),
		pending:   lf.NewQueue[Frame](),
	}, nil
}

const captureBatchSize = 128

/*
CaptureWriter batches frames into one transaction per batch.
*/
type CaptureWriter struct {
	store     *Store
	id        int64
	startedAt time.Time
	seq       int64
	committed []Frame
	pending   *lf.Queue[Frame]
	queued    atomic.Uint64
	draining  atomic.Bool
	failure   atomic.Pointer[captureFailure]
}

type captureFailure struct {
	err error
}

func (writer *CaptureWriter) ensureCapture() error {
	started := writer.startedAt

	if started.IsZero() {
		started = time.Now().UTC()
	}

	_, err := writer.store.database.Exec(
		"INSERT INTO captures (id, started_at, frames) VALUES (?, ?, 0) "+
			"ON CONFLICT(id) DO NOTHING",
		writer.id,
		started.Format(time.RFC3339Nano),
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"backtest: ensure capture record",
			err,
		))
	}

	return nil
}

/*
Write buffers one captured frame; the batch commits when full.
*/
func (writer *CaptureWriter) Write(frame Frame) error {
	if failure := writer.failure.Load(); failure != nil {
		return failure.err
	}

	writer.queued.Add(1)
	writer.pending.Enqueue(frame)
	return writer.drain()
}

func (writer *CaptureWriter) write(frame Frame) error {
	writer.committed = append(writer.committed, frame)

	if len(writer.committed) < captureBatchSize {
		return nil
	}

	return writer.flush()
}

/*
Flush commits the buffered batch in one transaction.
*/
func (writer *CaptureWriter) Flush() error {
	if err := writer.drain(); err != nil {
		return err
	}

	return writer.flush()
}

func (writer *CaptureWriter) drain() error {
	if !writer.draining.CompareAndSwap(false, true) {
		return nil
	}

	for {
		for {
			frame, found := writer.pending.Dequeue()

			if !found {
				break
			}

			writer.queued.Add(^uint64(0))

			if err := writer.write(frame); err != nil {
				writer.failure.Store(&captureFailure{err: err})
				writer.draining.Store(false)
				return err
			}
		}

		writer.draining.Store(false)

		if writer.queued.Load() == 0 ||
			!writer.draining.CompareAndSwap(false, true) {
			break
		}
	}

	if failure := writer.failure.Load(); failure != nil {
		return failure.err
	}

	return nil
}

func (writer *CaptureWriter) executeBatch() error {
	transaction, err := writer.store.database.Begin()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"backtest: begin capture batch",
			err,
		))
	}

	statement, err := transaction.Prepare(
		"INSERT INTO capture_frames (capture_id, seq, received_at, endpoint, payload) " +
			"VALUES (?, ?, ?, ?, ?)",
	)

	if err != nil {
		_ = transaction.Rollback()
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"backtest: prepare capture batch",
			err,
		))
	}

	for _, frame := range writer.committed {
		if _, err := statement.Exec(
			writer.id,
			writer.seq,
			frame.ReceivedAt.UTC().Format(time.RFC3339Nano),
			frame.Endpoint,
			frame.Payload,
		); err != nil {
			_ = statement.Close()
			_ = transaction.Rollback()
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"backtest: write capture frame",
				err,
			))
		}

		writer.seq++
	}

	if _, err := transaction.Exec(
		"UPDATE captures SET frames = frames + ? WHERE id = ?",
		len(writer.committed),
		writer.id,
	); err != nil {
		_ = statement.Close()
		_ = transaction.Rollback()
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"backtest: count capture frames",
			err,
		))
	}

	if err := statement.Close(); err != nil {
		_ = transaction.Rollback()
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"backtest: close capture batch",
			err,
		))
	}

	if err := transaction.Commit(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"backtest: commit capture batch",
			err,
		))
	}

	return nil
}

func (writer *CaptureWriter) flush() error {
	if len(writer.committed) == 0 {
		return nil
	}

	err := writer.executeBatch()

	if err != nil && isNoSuchTable(err) {
		if schemaErr := writer.store.EnsureSchema(); schemaErr != nil {
			return schemaErr
		}

		if captureErr := writer.ensureCapture(); captureErr != nil {
			return captureErr
		}

		err = writer.executeBatch()
	}

	if err != nil {
		return err
	}

	writer.committed = writer.committed[:0]
	return nil
}

/*
Capture satisfies the websocket CaptureSink interface: the live transport
hands every received frame to this method and the writer batches it.
*/
func (writer *CaptureWriter) Capture(
	endpoint string,
	payload []byte,
	receivedAt time.Time,
) error {
	return writer.Write(Frame{
		Endpoint: endpoint, Payload: payload, ReceivedAt: receivedAt,
	})
}

/*
Close flushes the final batch and stamps the run's end time.
*/
func (writer *CaptureWriter) Close() error {
	for writer.draining.Load() {
		runtime.Gosched()
	}

	if err := writer.Flush(); err != nil {
		return err
	}

	_, err := writer.store.database.Exec(
		"UPDATE captures SET ended_at = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339Nano),
		writer.id,
	)

	if err != nil && isNoSuchTable(err) {
		if schemaErr := writer.store.EnsureSchema(); schemaErr != nil {
			return schemaErr
		}

		if captureErr := writer.ensureCapture(); captureErr != nil {
			return captureErr
		}

		_, err = writer.store.database.Exec(
			"UPDATE captures SET ended_at = ? WHERE id = ?",
			time.Now().UTC().Format(time.RFC3339Nano),
			writer.id,
		)
	}

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"backtest: close capture",
			err,
		))
	}

	return nil
}

/*
WriteEvent records one analytical event — the curated audit stream. Only
decision moments belong here; transport frames live in the capture tables.
*/
func (store *Store) WriteEvent(kind string, payload []byte) error {
	_, err := store.database.Exec(
		"INSERT INTO audit_events (at, kind, payload) VALUES (?, ?, ?)",
		time.Now().UTC().Format(time.RFC3339Nano),
		kind,
		payload,
	)

	if err != nil && isNoSuchTable(err) {
		if schemaErr := store.EnsureSchema(); schemaErr != nil {
			return schemaErr
		}

		_, err = store.database.Exec(
			"INSERT INTO audit_events (at, kind, payload) VALUES (?, ?, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
			kind,
			payload,
		)
	}

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"backtest: write audit event",
			err,
		))
	}

	return nil
}

/*
Events streams analytical events in order.
*/
func (store *Store) Events() (
	func() (kind string, payload []byte, at time.Time, ok bool), error,
) {
	rows, err := store.database.Query(
		"SELECT kind, payload, at FROM audit_events ORDER BY id",
	)

	if err != nil && isNoSuchTable(err) {
		if schemaErr := store.EnsureSchema(); schemaErr != nil {
			return nil, schemaErr
		}

		rows, err = store.database.Query(
			"SELECT kind, payload, at FROM audit_events ORDER BY id",
		)
	}

	if err != nil {
		return nil, fmt.Errorf("backtest: open audit events: %w", err)
	}

	next := func() (string, []byte, time.Time, bool) {
		if !rows.Next() {
			rows.Close()
			return "", nil, time.Time{}, false
		}

		var kind string
		var payload []byte
		var at string

		if err := rows.Scan(&kind, &payload, &at); err != nil {
			rows.Close()
			return "", nil, time.Time{}, false
		}

		parsed, _ := time.Parse(time.RFC3339Nano, at)

		return kind, payload, parsed, true
	}

	return next, nil
}

/*
ListCaptures returns every recorded run, newest first.
*/
func (store *Store) ListCaptures() ([]CaptureInfo, error) {
	rows, err := store.database.Query(`
		SELECT id, started_at, ended_at, frames
		FROM captures
		ORDER BY id DESC
	`)

	if err != nil && isNoSuchTable(err) {
		if schemaErr := store.EnsureSchema(); schemaErr != nil {
			return nil, schemaErr
		}

		rows, err = store.database.Query(`
			SELECT id, started_at, ended_at, frames
			FROM captures
			ORDER BY id DESC
		`)
	}

	if err != nil {
		return nil, fmt.Errorf("backtest: list captures: %w", err)
	}

	defer rows.Close()

	captures := make([]CaptureInfo, 0)

	for rows.Next() {
		var info CaptureInfo
		var startedAt string
		var endedAt sql.NullString

		if err := rows.Scan(&info.ID, &startedAt, &endedAt, &info.Frames); err != nil {
			return nil, fmt.Errorf("backtest: scan capture: %w", err)
		}

		if info.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt); err != nil {
			return nil, fmt.Errorf("backtest: parse capture start: %w", err)
		}

		if endedAt.Valid && endedAt.String != "" {
			parsed, err := time.Parse(time.RFC3339Nano, endedAt.String)

			if err != nil {
				return nil, fmt.Errorf("backtest: parse capture end: %w", err)
			}

			info.EndedAt = &parsed
		}

		captures = append(captures, info)
	}

	return captures, rows.Err()
}

/*
Frames streams one capture's frames in arrival order from the given time
onward. An empty `from` replays from the beginning. The second return value
releases the streaming rows; the playback cursor occupies one pooled
connection for the whole stream, so a consumer that stops early must call it
to return that connection to the pool.
*/
func (store *Store) Frames(captureID int64, from time.Time) (
	func() (Frame, bool), func(), error,
) {
	rows, err := store.database.Query(
		"SELECT endpoint, payload, received_at FROM capture_frames "+
			"WHERE capture_id = ? AND received_at >= ? "+
			"ORDER BY capture_id, seq",
		captureID,
		from.UTC().Format(time.RFC3339Nano),
	)

	if err != nil && isNoSuchTable(err) {
		if schemaErr := store.EnsureSchema(); schemaErr != nil {
			return nil, nil, schemaErr
		}

		rows, err = store.database.Query(
			"SELECT endpoint, payload, received_at FROM capture_frames "+
				"WHERE capture_id = ? AND received_at >= ? "+
				"ORDER BY capture_id, seq",
			captureID,
			from.UTC().Format(time.RFC3339Nano),
		)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("backtest: open capture frames: %w", err)
	}

	next := func() (Frame, bool) {
		if !rows.Next() {
			rows.Close()
			return Frame{}, false
		}

		var frame Frame
		var receivedAt string

		if err := rows.Scan(&frame.Endpoint, &frame.Payload, &receivedAt); err != nil {
			rows.Close()
			return Frame{}, false
		}

		frame.ReceivedAt, _ = time.Parse(time.RFC3339Nano, receivedAt)

		return frame, true
	}

	closeRows := func() {
		_ = rows.Close()
	}

	return next, closeRows, nil
}

/*
Bounds returns the first and last frame arrival times of one capture, which
frame the scrub slider's range.
*/
func (store *Store) Bounds(captureID int64) (time.Time, time.Time, error) {
	var first, last sql.NullString

	err := store.database.QueryRow(`
		SELECT
			(SELECT MIN(received_at) FROM capture_frames WHERE capture_id = ?),
			(SELECT MAX(received_at) FROM capture_frames WHERE capture_id = ?)
	`, captureID, captureID).Scan(&first, &last)

	if err != nil && isNoSuchTable(err) {
		if schemaErr := store.EnsureSchema(); schemaErr != nil {
			return time.Time{}, time.Time{}, schemaErr
		}

		err = store.database.QueryRow(`
			SELECT
				(SELECT MIN(received_at) FROM capture_frames WHERE capture_id = ?),
				(SELECT MAX(received_at) FROM capture_frames WHERE capture_id = ?)
		`, captureID, captureID).Scan(&first, &last)
	}

	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("backtest: capture bounds: %w", err)
	}

	if !first.Valid || !last.Valid {
		return time.Time{}, time.Time{}, fmt.Errorf("backtest: capture %d has no frames", captureID)
	}

	startedAt, err := time.Parse(time.RFC3339Nano, first.String)

	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("backtest: parse capture bound: %w", err)
	}

	endedAt, err := time.Parse(time.RFC3339Nano, last.String)

	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("backtest: parse capture bound: %w", err)
	}

	return startedAt, endedAt, nil
}

func isNoSuchTable(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), "no such table")
}
