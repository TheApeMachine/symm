package store

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWriterCapture(t *testing.T) {
	Convey("Given a writer over an in-memory store", t, func() {
		engine := newRecordingRepository()
		writer := NewWriter(engine)

		Convey("Capturing a frame writes it under its channel/feed kind with its endpoint", func() {
			err := writer.Capture("ticker", "wss://example", []byte("raw"), time.Now())

			So(err, ShouldBeNil)
			So(engine.kinds, ShouldResemble, []string{"ticker"})
			So(engine.endpoints, ShouldResemble, []string{"wss://example"})

			// The payload is the exact bytes off the wire; nothing is prefixed.
			So(string(engine.payloads[0]), ShouldEqual, "raw")
		})

		Convey("Capturing with a nil repository is a no-op", func() {
			writer := NewWriter(nil)

			So(writer.Capture("ticker", "x", []byte("y"), time.Now()), ShouldBeNil)
		})
	})
}

/*
recordingRepository is a minimal in-memory Repository used to assert the writer
records the right kind, endpoint, and payload without a SQLite file.
*/
type recordingRepository struct {
	kinds     []string
	payloads  [][]byte
	endpoints []string
}

func newRecordingRepository() *recordingRepository {
	return &recordingRepository{}
}

func (repo *recordingRepository) WriteFrame(endpoint, kind string, payload []byte, at time.Time) error {
	repo.kinds = append(repo.kinds, kind)
	repo.payloads = append(repo.payloads, payload)
	repo.endpoints = append(repo.endpoints, endpoint)
	return nil
}

func (repo *recordingRepository) Close() error { return nil }
