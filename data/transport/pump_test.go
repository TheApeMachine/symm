package transport

import (
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func newTestPump(pipeline io.ReadWriteCloser) *Pump {
	return &Pump{
		pipeline: pipeline,
	}
}

func TestPumpRead(t *testing.T) {
	Convey("Given a pump over a pipeline", t, func() {
		pipeline := newTestBuffer([]byte("pump data"))
		pump := newTestPump(pipeline)

		buffer := make([]byte, 16)
		n, err := pump.Read(buffer)

		Convey("Then it should delegate to the pipeline", func() {
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len("pump data"))
			So(string(buffer[:n]), ShouldEqual, "pump data")
		})
	})
}

func TestPumpWrite(t *testing.T) {
	Convey("Given a pump over a pipeline", t, func() {
		pipeline := newTestBuffer(nil)
		pump := newTestPump(pipeline)

		payload := []byte("written")
		n, err := pump.Write(payload)

		Convey("Then it should delegate to the pipeline", func() {
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len(payload))
			So(pipeline.String(), ShouldEqual, "written")
		})
	})
}

func TestPumpClose(t *testing.T) {
	Convey("Given a pump over a pipeline", t, func() {
		pipeline := newTestBuffer(nil)
		pump := newTestPump(pipeline)

		Convey("When closing", func() {
			So(pump.Close(), ShouldBeNil)
		})
	})
}

func BenchmarkPumpRead(b *testing.B) {
	payload := []byte("pump benchmark payload")

	b.ResetTimer()

	for b.Loop() {
		pipeline := newTestBuffer(payload)
		pump := newTestPump(pipeline)
		buffer := make([]byte, len(payload))

		if _, err := pump.Read(buffer); err != nil && err != io.EOF {
			b.Fatal(err)
		}
	}
}