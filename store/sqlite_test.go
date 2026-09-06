package store

import (
	"bytes"
	"database/sql"
	"testing"
	"time"

	"github.com/phuslu/log"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
)

func TestNewSQLite(t *testing.T) {
	Convey("Given a store package", t, func() {
		Convey("Constructing with an empty path", func() {
			engine, err := NewSQLite("")

			So(engine, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

		Convey("Constructing with a valid file path", func() {
			path := t.TempDir() + "/events.sqlite"
			engine, err := NewSQLite(path)

			So(err, ShouldBeNil)
			So(engine, ShouldNotBeNil)

			Reset(func() {
				_ = engine.Close()
			})

			Convey("Writing and reading back one frame round-trips the bytes and endpoint", func() {
				at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
				err := engine.WriteFrame("wss://example/ws", "ticker", []byte("hello"), at)

				So(err, ShouldBeNil)

				rows, queryErr := engine.database.Query(
					"SELECT kind, endpoint, data FROM events WHERE kind = 'ticker'",
				)

				So(queryErr, ShouldBeNil)

				defer rows.Close()

				var kind string
				var endpoint string
				var data []byte
				found := rows.Next()

				So(found, ShouldBeTrue)

				So(rows.Scan(&kind, &endpoint, &data), ShouldBeNil)
				So(kind, ShouldEqual, "ticker")
				So(endpoint, ShouldEqual, "wss://example/ws")
				So(string(data), ShouldEqual, "hello")
			})

			Convey("Only the capture-order index is retained for event reads", func() {
				var obsolete int
				So(engine.database.QueryRow(
					`SELECT count(*) FROM sqlite_master
					 WHERE type = 'index'
					 AND name IN (
						'idx_events_kind', 'idx_events_at', 'idx_events_endpoint', 'idx_events_run_kind'
					 )`,
				).Scan(&obsolete), ShouldBeNil)
				So(obsolete, ShouldEqual, 0)

				var marketIndex int
				So(engine.database.QueryRow(
					`SELECT count(*) FROM sqlite_master
					 WHERE type = 'index' AND name = 'idx_events_market_run_capture'`,
				).Scan(&marketIndex), ShouldBeNil)
				So(marketIndex, ShouldEqual, 1)
			})

			Convey("An identity-addressed read never scans the capture tape", func() {
				var identityIndex int
				So(engine.database.QueryRow(
					`SELECT count(*) FROM sqlite_master
					 WHERE type = 'index' AND name = 'idx_events_run_capture'`,
				).Scan(&identityIndex), ShouldBeNil)
				So(identityIndex, ShouldEqual, 1)

				/*
					The partial market index cannot serve a query that does not
					constrain kind, so without the identity index above this plan
					degrades to a full scan of every captured frame — which is what
					held the writer's connection and stalled capture.
				*/
				var plan string
				So(engine.database.QueryRow(
					`EXPLAIN QUERY PLAN
					 SELECT run_id, capture_seq FROM events
					 WHERE run_id = 'r' AND capture_seq > 0
					 ORDER BY capture_seq ASC LIMIT 8`,
				).Scan(new(int), new(int), new(int), &plan), ShouldBeNil)
				So(plan, ShouldContainSubstring, "idx_events_run_capture")
				So(plan, ShouldNotContainSubstring, "SCAN events")
			})

			Convey("Inspection reads never share the capture writer's connection", func() {
				So(engine.Reader(), ShouldNotBeNil)
				So(engine.Reader(), ShouldNotEqual, engine.database)

				Convey("And the inspection pool is read-only", func() {
					_, err := engine.Reader().Exec(
						`INSERT INTO events (kind, endpoint, at, data)
						 VALUES ('ticker', 'e', '2026-01-01T00:00:00Z', x'00')`,
					)
					So(err, ShouldNotBeNil)
				})
			})
		})
	})
}

func TestSQLiteEnsureSchema(t *testing.T) {
	Convey("Given a retained learning journal awaiting its new kind index", t, func() {
		engine, err := NewSQLite(t.TempDir() + "/events.sqlite")
		So(err, ShouldBeNil)
		Reset(func() { So(engine.Close(), ShouldBeNil) })
		_, err = engine.database.Exec(`INSERT INTO learning_events(run_id,symbol,at,data)
			VALUES ('retained','TEST/USD','2026-09-06T00:00:00Z',
			CAST('{"id":1,"kind":"issued","symbol":"TEST/USD"}' AS BLOB))`)
		So(err, ShouldBeNil)
		_, err = engine.database.Exec(`DROP INDEX learning_events_kind`)
		So(err, ShouldBeNil)

		var output bytes.Buffer
		previous := log.DefaultLogger
		log.DefaultLogger = log.Logger{Level: log.InfoLevel, Writer: log.IOWriter{Writer: &output}}
		Reset(func() { log.DefaultLogger = previous })

		Convey("Migration announces the scan and completion while preserving the journal", func() {
			So(engine.EnsureSchema(), ShouldBeNil)
			So(output.String(), ShouldContainSubstring, "building learning index learning_events_kind")
			So(output.String(), ShouldContainSubstring, "learning index learning_events_kind ready")
			So(output.String(), ShouldNotContainSubstring, "building learning index learning_events_candidate")
			var payload []byte
			So(engine.database.QueryRow(`SELECT data FROM learning_events`).Scan(&payload), ShouldBeNil)
			So(string(payload), ShouldEqual, `{"id":1,"kind":"issued","symbol":"TEST/USD"}`)

			Convey("The next startup reuses the completed index without another scan", func() {
				output.Reset()
				So(engine.EnsureSchema(), ShouldBeNil)
				So(output.String(), ShouldEqual, "")
				var plan string
				So(engine.database.QueryRow(`EXPLAIN QUERY PLAN SELECT id FROM learning_events
					WHERE json_extract(data,'$.kind')='portfolio_resolved' ORDER BY id DESC LIMIT 1`).
					Scan(new(int), new(int), new(int), &plan), ShouldBeNil)
				So(plan, ShouldContainSubstring, "learning_events_kind")
			})
		})
	})
}

func TestSQLiteWriteFrame(t *testing.T) {
	Convey("Given an open sqlite engine", t, func() {
		path := t.TempDir() + "/events.sqlite"
		engine, err := NewSQLite(path)
		So(err, ShouldBeNil)

		Reset(func() {
			_ = engine.Close()
		})

		Convey("Writing multiple kinds keeps them distinct by kind", func() {
			So(engine.WriteFrame("wss://a", "ticker", []byte("1"), time.Now()), ShouldBeNil)
			So(engine.WriteFrame("wss://b", "trade", []byte("2"), time.Now()), ShouldBeNil)
			So(engine.WriteFrame("wss://a", "ticker", []byte("3"), time.Now()), ShouldBeNil)

			rows, queryErr := engine.database.Query(
				"SELECT kind FROM events ORDER BY id",
			)

			So(queryErr, ShouldBeNil)

			defer rows.Close()

			kinds := make([]string, 0, 3)

			for rows.Next() {
				var kind string
				So(rows.Scan(&kind), ShouldBeNil)
				kinds = append(kinds, kind)
			}

			So(kinds, ShouldResemble, []string{"ticker", "trade", "ticker"})
		})
	})
}

func TestSQLiteEndpointMigration(t *testing.T) {
	Convey("Given a database created before the endpoint column split", t, func() {
		path := t.TempDir() + "/events.sqlite"

		rawDatabase, err := sql.Open("sqlite3", path)
		So(err, ShouldBeNil)

		_, err = rawDatabase.Exec(`
CREATE TABLE events (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    at   TEXT NOT NULL,
    data BLOB NOT NULL
);`)
		So(err, ShouldBeNil)

		So(rawDatabase.Close(), ShouldBeNil)

		Convey("Opening it through NewSQLite adds the endpoint column without losing rows", func() {
			engine, err := NewSQLite(path)
			So(err, ShouldBeNil)

			Reset(func() {
				_ = engine.Close()
			})

			So(engine.WriteFrame("", "legacy", []byte("kept"), time.Now()), ShouldBeNil)

			var endpoint string

			So(engine.database.QueryRow(
				"SELECT endpoint FROM events WHERE kind = 'legacy'",
			).Scan(&endpoint), ShouldBeNil)

			// New records default to an empty endpoint, not a welded URL.
			So(endpoint, ShouldEqual, "")
		})
	})
}

func TestSQLiteWriteRunAndCapture(t *testing.T) {
	Convey("Given an open sqlite engine", t, func() {
		path := t.TempDir() + "/events.sqlite"
		engine, err := NewSQLite(path)
		So(err, ShouldBeNil)

		Reset(func() {
			_ = engine.Close()
		})

		Convey("A run record and a captured frame round-trip their identities", func() {
			run := hindsight.Run{
				ID:           "run-abc",
				StartedAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				ConfigDigest: "digest-1",
			}

			So(engine.WriteRun(run), ShouldBeNil)

			identity := hindsight.CaptureIdentity{
				Run:            "run-abc",
				Sequence:       9,
				Stream:         "wss://example:ticker",
				StreamEpoch:    2,
				StreamSequence: 7,
			}

			So(engine.WriteCapture(identity, "wss://example", "ticker", []byte("payload"), time.Now()), ShouldBeNil)

			var (
				runID        string
				configDigest string
			)

			So(engine.database.QueryRow(
				"SELECT id, config_digest FROM runs WHERE id = 'run-abc'",
			).Scan(&runID, &configDigest), ShouldBeNil)

			So(runID, ShouldEqual, "run-abc")
			So(configDigest, ShouldEqual, "digest-1")

			var (
				runValue  string
				seq       uint64
				stream    string
				epoch     uint64
				streamSeq uint64
			)

			So(engine.database.QueryRow(
				`SELECT run_id, capture_seq, stream, stream_epoch, stream_seq
				 FROM events WHERE kind = 'ticker'`,
			).Scan(&runValue, &seq, &stream, &epoch, &streamSeq), ShouldBeNil)

			So(runValue, ShouldEqual, "run-abc")
			So(seq, ShouldEqual, uint64(9))
			So(stream, ShouldEqual, "wss://example:ticker")
			So(epoch, ShouldEqual, uint64(2))
			So(streamSeq, ShouldEqual, uint64(7))
		})

		Convey("WriteCapture rejects an invalid identity", func() {
			err := engine.WriteCapture(hindsight.CaptureIdentity{}, "wss://example", "ticker", []byte("x"), time.Now())
			So(err, ShouldNotBeNil)
		})

		Convey("Compressible capture remains byte-exact on identity read", func() {
			identity := hindsight.CaptureIdentity{
				Run: "run-compressed", Sequence: 1, Stream: "public:ticker", StreamEpoch: 1,
			}
			payload := bytes.Repeat([]byte(`{"channel":"ticker","data":[]}`), 256)
			So(engine.WriteCapture(
				identity, "wss://example", "ticker", payload, time.Now(),
			), ShouldBeNil)

			var encoding string
			So(engine.database.QueryRow(
				"SELECT encoding FROM events WHERE run_id = ? AND capture_seq = ?",
				string(identity.Run), uint64(identity.Sequence),
			).Scan(&encoding), ShouldBeNil)
			So(encoding, ShouldEqual, "zstd")

			decoded, err := engine.ReadCapture(identity)
			So(err, ShouldBeNil)
			So(decoded, ShouldResemble, payload)
		})
	})
}

func TestSQLiteMarkGappedTest(t *testing.T) {
	Convey("Given an open sqlite engine with a COMPLETE run", t, func() {
		path := t.TempDir() + "/events.sqlite"
		engine, err := NewSQLite(path)
		So(err, ShouldBeNil)

		Reset(func() {
			_ = engine.Close()
		})

		run := hindsight.Run{
			ID:        "run-1",
			StartedAt: time.Now(),
			Integrity: hindsight.IntegrityComplete,
		}
		So(engine.WriteRun(run), ShouldBeNil)

		Convey("MarkGapped flips the run to GAPPED and persists a Gap row", func() {
			So(engine.MarkGapped("run-1", 42, "capture_persistence_failure", "boom"), ShouldBeNil)

			var integrity string
			So(engine.database.QueryRow(
				"SELECT integrity FROM runs WHERE id = 'run-1'",
			).Scan(&integrity), ShouldBeNil)
			So(integrity, ShouldEqual, "GAPPED")

			var (
				encoding string
				sequence uint64
				detail   string
			)
			So(engine.database.QueryRow(
				"SELECT encoding, sequence, detail FROM gaps WHERE run_id = 'run-1'",
			).Scan(&encoding, &sequence, &detail), ShouldBeNil)
			So(encoding, ShouldEqual, "capture_persistence_failure")
			So(sequence, ShouldEqual, uint64(42))
			So(detail, ShouldEqual, "boom")
		})
	})
}

func TestSQLiteLifecycleEventTest(t *testing.T) {
	Convey("Given an open sqlite engine", t, func() {
		path := t.TempDir() + "/events.sqlite"
		engine, err := NewSQLite(path)
		So(err, ShouldBeNil)

		Reset(func() {
			_ = engine.Close()
		})

		Convey("a lifecycle event round-trips with its decision correlation", func() {
			So(engine.WriteRun(hindsight.Run{ID: "run-1", StartedAt: time.Now()}), ShouldBeNil)

			So(engine.WriteLifecycleEvent("run-1", hindsight.LifecycleEvent{
				DecisionID: "entry-1",
				Symbol:     "XBT/USD",
				Kind:       "position_open",
				Action:     "enter",
				At:         time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC),
			}), ShouldBeNil)

			events, err := engine.ListLifecycleEvents("run-1")
			So(err, ShouldBeNil)
			So(len(events), ShouldEqual, 1)
			So(events[0].DecisionID, ShouldEqual, "entry-1")
			So(events[0].Symbol, ShouldEqual, "XBT/USD")
			So(events[0].Kind, ShouldEqual, "position_open")
		})

		Convey("an event without a decision id or kind is rejected", func() {
			So(engine.WriteLifecycleEvent("run-1", hindsight.LifecycleEvent{Symbol: "XBT/USD"}), ShouldNotBeNil)
		})
	})
}

func BenchmarkSQLiteWriteCapture(b *testing.B) {
	engine, err := NewSQLite(b.TempDir() + "/events.sqlite")

	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() {
		_ = engine.Close()
	})

	payload := bytes.Repeat(
		[]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","bid":100,"ask":101}]}`),
		8,
	)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		sequence := hindsight.CaptureSequence(b.N)
		identity := hindsight.CaptureIdentity{
			Run: "benchmark", Sequence: sequence, Stream: "public:ticker", StreamEpoch: 1,
		}

		if err := engine.WriteCapture(
			identity, "wss://example", "ticker", payload, time.Unix(0, int64(sequence)),
		); err != nil {
			b.Fatal(err)
		}
	}
}
