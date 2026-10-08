package data_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestBatch(t *testing.T) {
	Convey("Given a Batch primitive with size 2, stride 1", t, func() {
		batch := data.NewBatch(2, 1)

		Convey("It yields overlapping batches as Value primitives", func() {
			var batches []core.Primitive

			for ptr := range batch.Next(data.NewValue(1.0, 2.0, 3.0).Next(nil)) {
				chunk := (*data.Value[unsafe.Pointer])(ptr)
				batches = append(batches, chunk)
			}

			So(len(batches), ShouldEqual, 2)
		})
	})
}
