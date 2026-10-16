package transport

import (
	"bytes"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type writeCountingBuffer struct {
	bytes.Buffer
	writes int
}

func (b *writeCountingBuffer) Write(p []byte) (int, error) {
	b.writes++
	return b.Buffer.Write(p)
}

type completeFrameReader struct {
	frame []byte
}

func (reader *completeFrameReader) Read(payload []byte) (int, error) {
	written := copy(payload, reader.frame)

	if written < len(reader.frame) {
		return written, io.ErrShortBuffer
	}

	return written, io.EOF
}

func TestCopyRetriesCompleteFrameOnShortBuffer(t *testing.T) {
	Convey("Given a complete-frame reader larger than the initial copy buffer", t, func() {
		reader := &completeFrameReader{
			frame: bytes.Repeat([]byte("x"), copyBufferSize+1),
		}
		writer := &writeCountingBuffer{}

		Convey("When copying the frame", func() {
			written, err := Copy(writer, reader)

			Convey("Then the destination should receive one complete frame", func() {
				So(err, ShouldBeNil)
				So(written, ShouldEqual, len(reader.frame))
				So(writer.writes, ShouldEqual, 1)
				So(writer.Bytes(), ShouldResemble, reader.frame)
			})
		})
	})
}
