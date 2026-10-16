package transport

import (
	"bytes"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewGateWriter(t *testing.T) {
	Convey("Given a destination writer", t, func() {
		destination := bytes.NewBuffer(nil)
		gate := NewGateWriter(destination, nil)

		Convey("Then the gate should wrap the writer", func() {
			So(gate, ShouldNotBeNil)
			So(gate.W, ShouldEqual, destination)
		})
	})
}

func TestGateWriterWrite(t *testing.T) {
	Convey("Given a gate without transform", t, func() {
		destination := bytes.NewBuffer(nil)
		gate := NewGateWriter(destination, nil)

		payload := []byte("forwarded")
		n, err := gate.Write(payload)

		Convey("Then bytes should pass through", func() {
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len(payload))
			So(destination.String(), ShouldEqual, "forwarded")
		})
	})

	Convey("Given a gate that drops frames", t, func() {
		destination := bytes.NewBuffer(nil)
		gate := NewGateWriter(destination, func(p []byte) []byte {
			return nil
		})

		n, err := gate.Write([]byte("dropped"))

		Convey("Then write should succeed without forwarding", func() {
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len("dropped"))
			So(destination.Len(), ShouldEqual, 0)
		})
	})

	Convey("Given a gate that transforms frames", t, func() {
		destination := bytes.NewBuffer(nil)
		gate := NewGateWriter(destination, func(p []byte) []byte {
			return []byte("mapped")
		})

		n, err := gate.Write([]byte("original"))

		Convey("Then transformed bytes should be written", func() {
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len("original"))
			So(destination.String(), ShouldEqual, "mapped")
		})
	})

	Convey("Given a nil gate", t, func() {
		var gate *GateWriter
		n, err := gate.Write([]byte("x"))

		Convey("Then write should fail closed", func() {
			So(err, ShouldEqual, io.ErrClosedPipe)
			So(n, ShouldEqual, 0)
		})
	})
}

func TestGateWriterRead(t *testing.T) {
	Convey("Given a gate over a readable writer", t, func() {
		destination := newTestBuffer([]byte("readable"))
		gate := NewGateWriter(destination, nil)

		buffer := make([]byte, 16)
		n, err := gate.Read(buffer)

		Convey("Then it should delegate reads", func() {
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len("readable"))
			So(string(buffer[:n]), ShouldEqual, "readable")
		})
	})

	Convey("Given a gate over a write-only writer", t, func() {
		gate := NewGateWriter(bytes.NewBuffer(nil), nil)
		buffer := make([]byte, 8)
		n, err := gate.Read(buffer)

		Convey("Then read should return EOF", func() {
			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, 0)
		})
	})
}

func BenchmarkGateWriterWrite(b *testing.B) {
	payload := []byte("benchmark frame")

	b.ResetTimer()

	for b.Loop() {
		destination := bytes.NewBuffer(make([]byte, 0, len(payload)))
		gate := NewGateWriter(destination, func(p []byte) []byte {
			return p
		})

		if _, err := gate.Write(payload); err != nil {
			b.Fatal(err)
		}
	}
}
