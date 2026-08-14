package audit

import (
	"io"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewReader(t *testing.T) {
	Convey("Given compressed recorder output", t, func() {
		path := filepath.Join(t.TempDir(), "market-frames.jsonl.zst")
		recorder, err := NewRecorder(path)
		So(err, ShouldBeNil)
		So(recorder.Write(map[string]any{"sequence": 1}), ShouldBeNil)
		So(recorder.Close(), ShouldBeNil)

		reader, err := NewReader(path)
		So(err, ShouldBeNil)
		decoded, err := io.ReadAll(reader)
		So(err, ShouldBeNil)
		So(reader.Close(), ShouldBeNil)

		Convey("It should transparently stream the original JSONL", func() {
			So(string(decoded), ShouldEqual, "{\"sequence\":1}\n")
		})
	})
}

func TestReaderRead(t *testing.T) {
	Convey("Given plain recorder output", t, func() {
		path := filepath.Join(t.TempDir(), "runtime-audit.jsonl")
		recorder, err := NewRecorder(path)
		So(err, ShouldBeNil)
		So(recorder.Write(map[string]any{"sequence": 2}), ShouldBeNil)
		So(recorder.Close(), ShouldBeNil)

		reader, err := NewReader(path)
		So(err, ShouldBeNil)
		decoded, err := io.ReadAll(reader)
		So(err, ShouldBeNil)
		So(reader.Close(), ShouldBeNil)

		Convey("It should preserve the plain JSONL path", func() {
			So(string(decoded), ShouldEqual, "{\"sequence\":2}\n")
		})
	})
}

func TestReaderClose(t *testing.T) {
	Convey("Given an open capture reader", t, func() {
		path := filepath.Join(t.TempDir(), "runtime-audit.jsonl")
		recorder, err := NewRecorder(path)
		So(err, ShouldBeNil)
		So(recorder.Close(), ShouldBeNil)
		reader, err := NewReader(path)
		So(err, ShouldBeNil)

		Convey("It should close its file", func() {
			So(reader.Close(), ShouldBeNil)
			_, err = reader.Read(make([]byte, 1))
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkReaderRead(b *testing.B) {
	path := filepath.Join(b.TempDir(), "market-frames.jsonl.zst")
	recorder, err := NewRecorder(path)

	if err != nil {
		b.Fatal(err)
	}

	if err = recorder.Write(map[string]any{"payload": "market frame"}); err != nil {
		b.Fatal(err)
	}

	if err = recorder.Close(); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		reader, openErr := NewReader(path)

		if openErr != nil {
			b.Fatal(openErr)
		}

		if _, readErr := io.Copy(io.Discard, reader); readErr != nil {
			b.Fatal(readErr)
		}

		if closeErr := reader.Close(); closeErr != nil {
			b.Fatal(closeErr)
		}
	}
}
