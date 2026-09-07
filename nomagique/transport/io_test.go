package transport

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestIOAppendRun(t *testing.T) {
	Convey("Given a reusable run buffer", t, func() {
		var buffer IO
		Convey("It appends deliveries and reports terminal errors to its owner", func() {
			marker := errors.New("terminal failure")
			var source *Generator[float64]
			source = NewGenerator(func(yield func(float64) bool) {
				yield(2)
				source.Error(marker)
			})
			err := buffer.appendRun(source, nil)
			So(errors.Is(err, marker), ShouldBeTrue)
			So(buffer.Error(), ShouldBeNil)
			So(core.To[float64](buffer.Next(nil)), ShouldEqual, 2)
			So(buffer.Next(nil), ShouldBeNil)
		})
		Convey("Appending another run preserves order without decoding carriers", func() {
			first, second := core.From(2.0), core.From(3.0)
			So(buffer.appendRun(NewIO(first), nil), ShouldBeNil)
			So(buffer.appendRun(NewIO(second), nil), ShouldBeNil)
			So(buffer.Next(nil), ShouldEqual, first)
			So(buffer.Next(nil), ShouldEqual, second)
			So(buffer.Next(nil), ShouldBeNil)
		})
	})
}

func TestIOReset(t *testing.T) {
	Convey("Given a drained buffer with a saved public snapshot", t, func() {
		first := core.From(2.0)
		buffer := NewIO(first)
		snapshot := buffer.Read().([]core.Primitive)
		storage := buffer.values
		capacity := cap(storage)
		buffer.Error(errors.New("old run"))
		buffer.reset()

		Convey("Reset releases references but retains capacity and saved results", func() {
			So(storage[0], ShouldBeNil)
			So(cap(buffer.values), ShouldEqual, capacity)
			So(buffer.Error(), ShouldBeNil)
			So(buffer.Next(nil), ShouldBeNil)
			So(snapshot[0], ShouldEqual, first)
		})
	})
}
