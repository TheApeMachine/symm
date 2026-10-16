package transport

import (
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewSink(t *testing.T) {
	Convey("Given a need for a new Sink", t, func() {
		Convey("When creating a new Sink", func() {
			sink := NewSink()

			Convey("Then it should not be nil", func() {
				So(sink, ShouldNotBeNil)
			})

			Convey("Then it should implement io.ReadWriteCloser", func() {
				_, isReader := any(sink).(io.Reader)
				_, isWriter := any(sink).(io.Writer)
				_, isCloser := any(sink).(io.Closer)

				So(isReader, ShouldBeTrue)
				So(isWriter, ShouldBeTrue)
				So(isCloser, ShouldBeTrue)
			})
		})
	})
}

func TestSinkRead(t *testing.T) {
	Convey("Given a Sink instance", t, func() {
		sink := NewSink()

		Convey("When reading from the sink", func() {
			buf := make([]byte, 10)
			n, err := sink.Read(buf)

			Convey("Then it should return EOF", func() {
				So(err, ShouldEqual, io.EOF)
			})

			Convey("Then it should not read any bytes", func() {
				So(n, ShouldEqual, 0)
			})
		})
	})
}

func TestSinkWrite(t *testing.T) {
	Convey("Given a Sink instance", t, func() {
		sink := NewSink()

		Convey("When writing to the sink", func() {
			data := []byte("test data")
			n, err := sink.Write(data)

			Convey("Then it should not return an error", func() {
				So(err, ShouldBeNil)
			})

			Convey("Then it should report the correct number of bytes written", func() {
				So(n, ShouldEqual, len(data))
			})
		})

		Convey("When writing empty data to the sink", func() {
			data := []byte{}
			n, err := sink.Write(data)

			Convey("Then it should not return an error", func() {
				So(err, ShouldBeNil)
			})

			Convey("Then it should report zero bytes written", func() {
				So(n, ShouldEqual, 0)
			})
		})

		Convey("When writing nil data to the sink", func() {
			var data []byte
			n, err := sink.Write(data)

			Convey("Then it should not return an error", func() {
				So(err, ShouldBeNil)
			})

			Convey("Then it should report zero bytes written", func() {
				So(n, ShouldEqual, 0)
			})
		})
	})
}

func TestSinkClose(t *testing.T) {
	Convey("Given a Sink instance", t, func() {
		sink := NewSink()

		Convey("When closing the sink", func() {
			err := sink.Close()

			Convey("Then it should not return an error", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When using the sink after closing", func() {
			sink.Close()

			Convey("Then reading should still work normally", func() {
				buf := make([]byte, 10)
				n, err := sink.Read(buf)
				So(err, ShouldEqual, io.EOF)
				So(n, ShouldEqual, 0)
			})

			Convey("Then writing should still work normally", func() {
				n, err := sink.Write([]byte("test"))
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 4)
			})
		})
	})
}

func BenchmarkSinkWrite(b *testing.B) {
	payload := []byte("sink benchmark payload")

	b.ResetTimer()

	for b.Loop() {
		sink := NewSink()

		if _, err := sink.Write(payload); err != nil {
			b.Fatal(err)
		}
	}
}
