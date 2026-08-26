package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestNewRecorder(t *testing.T) {
	Convey("Given a compressed JSONL capture path", t, func() {
		path := filepath.Join(t.TempDir(), "market-frames.jsonl.zst")

		recorder, err := NewRecorder(path)

		Convey("It should construct a single-consumer Zstandard writer", func() {
			So(err, ShouldBeNil)
			So(recorder.encoder, ShouldNotBeNil)
			So(recorder.Close(), ShouldBeNil)
		})
	})
}

func TestRecorderWrite(t *testing.T) {
	Convey("Given successive market capture rows", t, func() {
		path := filepath.Join(t.TempDir(), "market-frames.jsonl.zst")
		recorder, err := NewRecorder(path)
		So(err, ShouldBeNil)

		for sequence := range 1000 {
			So(recorder.Write(map[string]any{
				"sequence": sequence,
				"payload":  "repeated market payload",
			}), ShouldBeNil)
		}

		So(recorder.Close(), ShouldBeNil)
		file, err := os.Open(path)
		So(err, ShouldBeNil)
		defer file.Close()
		reader, err := zstd.NewReader(file)
		So(err, ShouldBeNil)
		defer reader.Close()
		decoder := json.NewDecoder(bufio.NewReader(reader))
		rows := 0

		for {
			var row map[string]any
			err = decoder.Decode(&row)

			if errors.Is(err, io.EOF) {
				break
			}

			So(err, ShouldBeNil)
			rows++
		}

		Convey("It should preserve every accepted JSONL row through compression", func() {
			So(rows, ShouldEqual, 1000)
		})
	})

	Convey("Given a sink-only recorder", t, func() {
		var writtenKind string
		var writtenPayload []byte
		recorder := &Recorder{
			EventSink: func(kind string, payload []byte) error {
				writtenKind = kind
				writtenPayload = payload
				return nil
			},
		}

		Convey("Write should route directly to EventSink", func() {
			err := recorder.Write(map[string]string{"foo": "bar"})
			So(err, ShouldBeNil)
			So(writtenKind, ShouldEqual, "metadata")
			So(string(writtenPayload), ShouldEqual, `{"foo":"bar"}`)
		})
	})
}

func TestRecorderClose(t *testing.T) {
	Convey("Given a recorder that has completed its stream", t, func() {
		recorder, err := NewRecorder(filepath.Join(t.TempDir(), "audit.jsonl"))
		So(err, ShouldBeNil)
		So(recorder.Close(), ShouldBeNil)

		err = recorder.Write(map[string]any{"late": true})
		var closed types.ClosedError

		Convey("It should reject writes after the close boundary", func() {
			So(errors.As(err, &closed), ShouldBeTrue)
		})
	})

	Convey("Given a sink-only recorder", t, func() {
		recorder := &Recorder{
			EventSink: func(kind string, payload []byte) error {
				return nil
			},
		}

		Convey("Close should succeed idempotently and reject subsequent writes", func() {
			So(recorder.Close(), ShouldBeNil)
			err := recorder.Write(map[string]string{"foo": "bar"})
			var closed types.ClosedError
			So(errors.As(err, &closed), ShouldBeTrue)
		})
	})
}

func BenchmarkRecorderWrite(b *testing.B) {
	recorder, err := NewRecorder(filepath.Join(b.TempDir(), "market-frames.jsonl.zst"))

	if err != nil {
		b.Fatal(err)
	}

	event := map[string]any{
		"endpoint": "public",
		"payload": map[string]any{
			"channel": "ticker",
			"data":    []map[string]string{{"symbol": "BTC/USD", "last": "63132.1"}},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err = recorder.Write(event); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()

	if err = recorder.Close(); err != nil {
		b.Fatal(err)
	}
}
