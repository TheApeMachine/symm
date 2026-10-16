package transport

import (
	"bytes"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFeedback(t *testing.T) {
	Convey("Given forward and backward writers", t, func() {
		forward := newTestBuffer(nil)
		backward := bytes.NewBuffer(nil)
		feedback := NewFeedback(forward, backward)

		Convey("Then feedback should be configured", func() {
			So(feedback, ShouldNotBeNil)
			So(feedback.forward, ShouldEqual, forward)
			So(feedback.backward, ShouldEqual, backward)
		})
	})
}

func TestFeedbackWrite(t *testing.T) {
	Convey("Given a feedback bridge", t, func() {
		forward := newTestBuffer(nil)
		backward := bytes.NewBuffer(nil)
		feedback := NewFeedback(forward, backward)

		payload := []byte("feedback payload")
		n, err := feedback.Write(payload)

		Convey("Then data should reach the forward writer", func() {
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len(payload))
			So(forward.String(), ShouldEqual, string(payload))
		})
	})
}

func TestFeedbackRead(t *testing.T) {
	Convey("Given a feedback bridge with forward data", t, func() {
		forward := newTestBuffer([]byte("forward data"))
		backward := bytes.NewBuffer(nil)
		feedback := NewFeedback(forward, backward)

		buffer := make([]byte, 32)
		n, err := feedback.Read(buffer)

		Convey("Then it should tee data to backward", func() {
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len("forward data"))
			So(string(buffer[:n]), ShouldEqual, "forward data")
			So(backward.String(), ShouldEqual, "forward data")
		})
	})

	Convey("Given an empty forward buffer", t, func() {
		feedback := NewFeedback(newTestBuffer(nil), bytes.NewBuffer(nil))
		buffer := make([]byte, 8)
		n, err := feedback.Read(buffer)

		Convey("Then it should return EOF", func() {
			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, 0)
		})
	})
}

func TestFeedbackClose(t *testing.T) {
	Convey("Given a feedback bridge with closable ends", t, func() {
		forward := newTestBuffer(nil)
		backward := newTestBuffer(nil)
		feedback := NewFeedback(forward, backward)

		Convey("When closing", func() {
			So(feedback.Close(), ShouldBeNil)
		})
	})
}

func BenchmarkFeedbackRead(b *testing.B) {
	payload := bytes.Repeat([]byte("f"), 1024)

	b.ResetTimer()

	for b.Loop() {
		forward := newTestBuffer(payload)
		backward := bytes.NewBuffer(make([]byte, 0, len(payload)))
		feedback := NewFeedback(forward, backward)
		buffer := make([]byte, len(payload))

		if _, err := feedback.Read(buffer); err != nil && err != io.EOF {
			b.Fatal(err)
		}
	}
}
