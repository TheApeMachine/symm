package store

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/types"
)

func TestWriterCapture(t *testing.T) {
	Convey("Given a writer over an in-memory store", t, func() {
		engine := newRecordingRepository()
		writer := NewWriter(engine)

		Convey("Capturing a frame writes a websocket_frame event", func() {
			err := writer.Capture("wss://example", []byte("raw"), time.Now())

			So(err, ShouldBeNil)
			So(engine.kinds, ShouldResemble, []string{FrameKind})
			So(string(engine.payloads[0][:len("wss://example")]), ShouldEqual, "wss://example")
		})

		Convey("Capturing with a nil repository is a no-op", func() {
			writer := NewWriter(nil)

			So(writer.Capture("x", []byte("y"), time.Now()), ShouldBeNil)
		})
	})
}

func TestLayerStep(t *testing.T) {
	Convey("Given a writer over an in-memory store", t, func() {
		engine := newRecordingRepository()
		writer := NewWriter(engine)
		layer := NewLayer(writer, "ticker.signals")

		Convey("Stepping an envelope records it under the layer name", func() {
			envelope := types.NewEnvelope(types.EnvelopeTicker)
			result := layer.Step(envelope)

			So(result, ShouldEqual, envelope)
			So(engine.kinds, ShouldResemble, []string{"ticker.signals"})
		})

		Convey("Stepping a nil envelope is a no-op", func() {
			layer.Step(nil)

			So(len(engine.kinds), ShouldEqual, 0)
		})
	})
}

/*
recordingRepository is a minimal in-memory Repository used to assert the writer
and layer record the right kinds and payloads without a SQLite file.
*/
type recordingRepository struct {
	kinds    []string
	payloads [][]byte
}

func newRecordingRepository() *recordingRepository {
	return &recordingRepository{}
}

func (repo *recordingRepository) WriteEvent(kind string, payload []byte, at time.Time) error {
	repo.kinds = append(repo.kinds, kind)
	repo.payloads = append(repo.payloads, payload)
	return nil
}

func (repo *recordingRepository) Close() error { return nil }
