package grid

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEmit(t *testing.T) {
	Convey("Given an Emit primitive configured for 3 regions", t, func() {
		emitter := NewEmit(3)

		Convey("Accumulates tokens and yields when batch size matches region count", func() {
			tokens := []uint64{101, 102, 103}
			emitted := data.Read[[]uint64](emitter.Next(data.NewValue(tokens...)))

			So(len(emitted), ShouldEqual, 3)
			So(emitted, ShouldResemble, []uint64{101, 102, 103})
			So(emitter.Error(), ShouldBeNil)
		})
	})
}
