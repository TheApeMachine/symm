package transport

import (
	"bytes"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type noOutputComponent struct{}

func (component *noOutputComponent) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (component *noOutputComponent) Write(p []byte) (int, error) {
	return len(p), nil
}

func (component *noOutputComponent) Close() error {
	return nil
}

type failingReadComponent struct{}

func (component *failingReadComponent) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (component *failingReadComponent) Write(p []byte) (int, error) {
	return len(p), nil
}

func (component *failingReadComponent) Close() error {
	return nil
}

func TestPipeline(t *testing.T) {
	Convey("Given a pipeline with components that produce data", t, func() {
		c1 := newTestBuffer([]byte("data from first"))
		c2 := newTestBuffer(nil)
		c3 := bytes.NewBuffer([]byte{})

		pipeline := NewPipeline(c1, c2)
		n, err := io.Copy(c3, pipeline)

		Convey("When reading without writing first", func() {
			So(n, ShouldNotEqual, 0)
			So(err, ShouldBeNil)
			So(c3.String(), ShouldEqual, "data from first")
		})
	})

	Convey("Given two pipelines with components that produce data", t, func() {
		in1 := bytes.NewBuffer([]byte("data from first"))
		p1 := NewPipeline(newTestBuffer(nil))
		p2 := NewPipeline(newTestBuffer(nil))
		buf := bytes.NewBuffer([]byte{})

		pipeline := NewPipeline(p1, p2)

		n, err := io.Copy(pipeline, in1)
		So(err, ShouldBeNil)
		So(n, ShouldEqual, len("data from first"))

		n, err = io.Copy(buf, pipeline)

		So(err, ShouldBeNil)
		So(n, ShouldEqual, len("data from first"))
		So(buf.String(), ShouldEqual, "data from first")
	})
}

func TestPipelineNoOutputStage(t *testing.T) {
	Convey("Given a pipeline stage with no output", t, func() {
		pipeline := NewPipeline(&noOutputComponent{}, newTestBuffer(nil))
		buffer := make([]byte, 32)

		n, err := pipeline.Read(buffer)

		Convey("Then the pipeline should stop without a no-progress error", func() {
			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, 0)
		})
	})
}

func TestPipelineNoOutputStopsDownstream(t *testing.T) {
	Convey("Given a pipeline stage with no output before a failing stage", t, func() {
		pipeline := NewPipeline(&noOutputComponent{}, &failingReadComponent{})
		buffer := make([]byte, 32)

		n, err := pipeline.Read(buffer)

		Convey("Then it should not run downstream stages on empty state", func() {
			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, 0)
		})
	})
}

func TestPipelineNoOutputAfterIntermediateCopy(t *testing.T) {
	Convey("Given a later pipeline stage with no output", t, func() {
		pipeline := NewPipeline(newTestBuffer([]byte("intermediate")), &noOutputComponent{}, newTestBuffer(nil))
		buffer := bytes.Repeat([]byte{1}, 32)

		n, err := pipeline.Read(buffer)

		Convey("Then it should not report copied intermediate bytes as output", func() {
			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, 0)
			So(buffer, ShouldResemble, bytes.Repeat([]byte{1}, 32))
		})
	})
}

func TestRead(t *testing.T) {
	Convey("Given a pipeline with components that produce data", t, func() {
		c1 := newTestBuffer([]byte("data from first"))
		c2 := newTestBuffer(nil)
		c3 := make([]byte, c1.Len())

		pipeline := NewPipeline(c1, c2)
		n, err := pipeline.Read(c3)
		So(err, ShouldBeNil)
		So(n, ShouldEqual, len("data from first"))
		So(string(c3), ShouldEqual, "data from first")
	})
}

func TestWrite(t *testing.T) {
	Convey("Given a pipeline with components that produce data", t, func() {
		c1 := bytes.NewBuffer([]byte("data from first"))
		c2 := newTestBuffer(nil)
		c3 := newTestBuffer(nil)
		c4 := bytes.NewBuffer([]byte{})

		pipeline := NewPipeline(c2, c3)
		n, err := io.Copy(pipeline, c1)
		So(err, ShouldBeNil)
		So(n, ShouldEqual, len("data from first"))

		n, err = io.Copy(c4, pipeline)
		So(err, ShouldBeNil)
		So(n, ShouldEqual, len("data from first"))
		So(c4.String(), ShouldEqual, "data from first")
	})
}

func TestEmptyPipeline(t *testing.T) {
	Convey("Given an empty pipeline with no components", t, func() {
		pipeline := NewPipeline()
		buf := make([]byte, 10)

		Convey("When reading", func() {
			n, err := pipeline.Read(buf)
			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, 0)
		})

		Convey("When writing", func() {
			n, err := pipeline.Write([]byte("test"))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 4)
		})
	})
}

func TestLargeDataPipeline(t *testing.T) {
	Convey("Given a pipeline with large data transfer", t, func() {
		// Create 1MB of test data
		largeData := make([]byte, 1024*1024)
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		src := bytes.NewBuffer(largeData)
		intermediate := newTestBuffer(nil)
		dest := bytes.NewBuffer([]byte{})

		pipeline := NewPipeline(intermediate)

		Convey("When copying large data through pipeline", func() {
			n, err := io.Copy(pipeline, src)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len(largeData))

			n, err = io.Copy(dest, pipeline)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len(largeData))
			So(bytes.Equal(dest.Bytes(), largeData), ShouldBeTrue)
		})
	})
}

func TestMultipleWriteReadCycles(t *testing.T) {
	Convey("Given a pipeline with multiple write-read cycles", t, func() {
		buf1 := newTestBuffer(nil)
		buf2 := newTestBuffer(nil)
		pipeline := NewPipeline(buf1, buf2)

		Convey("When performing multiple write-read cycles", func() {
			testData := []string{
				"first message",
				"second message",
				"third message",
			}

			for _, data := range testData {
				n, err := pipeline.Write([]byte(data))
				So(err, ShouldBeNil)
				So(n, ShouldEqual, len(data))

				result := make([]byte, len(data))
				n, err = pipeline.Read(result)
				So(err, ShouldBeNil)
				So(n, ShouldEqual, len(data))
				So(string(result), ShouldEqual, data)
			}
		})
	})
}

func TestEdgeCases(t *testing.T) {
	Convey("Given a pipeline testing edge cases", t, func() {
		buf1 := newTestBuffer(nil)
		buf2 := newTestBuffer(nil)
		pipeline := NewPipeline(buf1, buf2)

		Convey("When writing empty data", func() {
			n, err := pipeline.Write([]byte{})
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 0)
		})

		Convey("When writing nil data", func() {
			n, err := pipeline.Write(nil)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 0)
		})

		Convey("When reading with zero-length buffer", func() {
			n, err := pipeline.Read([]byte{})
			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, 0)
		})

		Convey("When reading with nil buffer", func() {
			n, err := pipeline.Read(nil)
			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, 0)
		})
	})
}

func BenchmarkPipelineRead(b *testing.B) {
	payload := []byte("data from first")

	b.ResetTimer()

	for b.Loop() {
		c1 := newTestBuffer(payload)
		c2 := newTestBuffer(nil)
		c3 := bytes.NewBuffer(nil)
		pipeline := NewPipeline(c1, c2)

		if _, err := io.Copy(c3, pipeline); err != nil && err != io.EOF {
			b.Fatal(err)
		}
	}
}

func BenchmarkPipelineWrite(b *testing.B) {
	payload := []byte("pipeline write payload")

	b.ResetTimer()

	for b.Loop() {
		c1 := newTestBuffer(nil)
		c2 := newTestBuffer(nil)
		pipeline := NewPipeline(c1, c2)

		if _, err := pipeline.Write(payload); err != nil {
			b.Fatal(err)
		}
	}
}
