package backtest

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCaptureWriterCapture(t *testing.T) {
	Convey("Given public, private, and level3 transports sharing one live capture", t, func() {
		store, err := NewStore(filepath.Join(t.TempDir(), "symm.sqlite"))
		So(err, ShouldBeNil)
		defer func() { So(store.Close(), ShouldBeNil) }()
		writer, err := store.OpenCapture()
		So(err, ShouldBeNil)
		var writers sync.WaitGroup
		endpoints := []string{"public", "private", "level3"}
		writeErrors := make(chan error, len(endpoints)*captureBatchSize)

		for _, endpoint := range endpoints {
			writers.Add(1)

			go func() {
				defer writers.Done()

				for index := range captureBatchSize {
					writeErrors <- writer.Capture(
						endpoint,
						[]byte(endpoint),
						time.Unix(0, int64(index)).UTC(),
					)
				}
			}()
		}

		writers.Wait()
		close(writeErrors)

		for err := range writeErrors {
			So(err, ShouldBeNil)
		}

		So(writer.Close(), ShouldBeNil)
		captures, err := store.ListCaptures()
		So(err, ShouldBeNil)

		Convey("It should retain every frame in the single ordered capture", func() {
			So(captures, ShouldHaveLength, 1)
			So(captures[0].Frames, ShouldEqual, int64(len(endpoints)*captureBatchSize))
			So(captures[0].EndedAt, ShouldNotBeNil)
		})
	})
}

func TestStoreConcurrentListWhileStreaming(t *testing.T) {
	Convey("Given a capture store with recorded frames", t, func() {
		store, err := NewStore(filepath.Join(t.TempDir(), "symm.sqlite"))
		So(err, ShouldBeNil)
		defer func() { So(store.Close(), ShouldBeNil) }()

		writer, err := store.OpenCapture()
		So(err, ShouldBeNil)

		for index := range captureBatchSize {
			So(writer.Capture(
				"public",
				[]byte(`{"channel":"ticker"}`),
				time.Unix(0, int64(index)).UTC(),
			), ShouldBeNil)
		}

		So(writer.Close(), ShouldBeNil)

		captures, err := store.ListCaptures()
		So(err, ShouldBeNil)
		So(captures, ShouldHaveLength, 1)
		captureID := captures[0].ID

		Convey("It should list captures while a frame cursor is open", func() {
			frames, release, err := store.Frames(captureID, time.Time{})
			So(err, ShouldBeNil)
			defer release()

			frame, ok := frames()
			So(ok, ShouldBeTrue)
			So(frame.Endpoint, ShouldEqual, "public")

			listed, err := store.ListCaptures()
			So(err, ShouldBeNil)
			So(listed, ShouldHaveLength, 1)
			So(listed[0].Frames, ShouldEqual, int64(captureBatchSize))
		})
	})
}

func TestMigrateCaptureCountsBackfillsLegacyStore(t *testing.T) {
	Convey("Given a legacy capture store without a frame counter", t, func() {
		path := filepath.Join(t.TempDir(), "symm.sqlite")
		database, err := sql.Open("sqlite3", path)
		So(err, ShouldBeNil)

		_, err = database.Exec(`
			CREATE TABLE captures (
				id INTEGER PRIMARY KEY,
				started_at TEXT NOT NULL,
				ended_at TEXT
			) STRICT;

			CREATE TABLE capture_frames (
				capture_id INTEGER NOT NULL,
				seq INTEGER NOT NULL,
				received_at TEXT NOT NULL,
				endpoint TEXT NOT NULL,
				payload BLOB NOT NULL,
				PRIMARY KEY (capture_id, seq)
			) STRICT;

			INSERT INTO captures (id, started_at) VALUES (1, '2026-01-01T00:00:00Z');
			INSERT INTO capture_frames (capture_id, seq, received_at, endpoint, payload)
				VALUES (1, 0, '2026-01-01T00:00:00Z', 'public', X'0102');
		`)
		So(err, ShouldBeNil)
		So(database.Close(), ShouldBeNil)

		store, err := NewStore(path)
		So(err, ShouldBeNil)
		defer func() { So(store.Close(), ShouldBeNil) }()

		Convey("It should add the counter and backfill existing frames", func() {
			captures, err := store.ListCaptures()
			So(err, ShouldBeNil)
			So(captures, ShouldHaveLength, 1)
			So(captures[0].Frames, ShouldEqual, 1)
		})
	})
}

func TestStoreEnsureSchema(t *testing.T) {
	Convey("Given an active capture store whose tables are deleted", t, func() {
		path := filepath.Join(t.TempDir(), "symm.sqlite")
		store, err := NewStore(path)
		So(err, ShouldBeNil)
		defer func() { So(store.Close(), ShouldBeNil) }()

		writer, err := store.OpenCapture()
		So(err, ShouldBeNil)

		_, err = store.database.Exec(`
			DROP TABLE capture_frames;
			DROP TABLE captures;
			DROP TABLE audit_events;
		`)
		So(err, ShouldBeNil)

		Convey("Writing frames should regenerate tables and succeed", func() {
			for index := range captureBatchSize {
				So(writer.Capture(
					"public",
					[]byte(`{"channel":"ticker"}`),
					time.Unix(0, int64(index)).UTC(),
				), ShouldBeNil)
			}

			So(writer.Close(), ShouldBeNil)

			captures, err := store.ListCaptures()
			So(err, ShouldBeNil)
			So(captures, ShouldHaveLength, 1)
			So(captures[0].Frames, ShouldEqual, int64(captureBatchSize))

			So(store.WriteEvent("decision", []byte(`{"action":"buy"}`)), ShouldBeNil)
		})
	})
}

func BenchmarkCaptureWriterCapture(b *testing.B) {
	store, err := NewStore(filepath.Join(b.TempDir(), "symm.sqlite"))

	if err != nil {
		b.Fatal(err)
	}

	defer func() {
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
	}()

	writer, err := store.OpenCapture()

	if err != nil {
		b.Fatal(err)
	}

	payload := []byte("captured websocket frame")
	at := time.Unix(0, 1).UTC()
	b.ResetTimer()

	for b.Loop() {
		if err := writer.Capture("public", payload, at); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()

	if err := writer.Close(); err != nil {
		b.Fatal(err)
	}
}
