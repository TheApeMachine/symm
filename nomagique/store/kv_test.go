package store_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestKVNext(t *testing.T) {
	Convey("Given a KV store", t, func() {
		op := store.NewKV()
		val := 42.0
		storedVal := data.NewValue(val)

		Convey("When writing a message", func() {
			writeMsg := data.NewMessage(data.WRITE, "store", "BTC", storedVal)
			var writeYields []float64

			for ptr := range op.Next(writeMsg.Next(nil)) {
				writeYields = append(writeYields, *(*float64)(ptr))
			}

			So(len(writeYields), ShouldEqual, 1)
			So(writeYields[0], ShouldEqual, 42.0)

			Convey("When reading back the message", func() {
				readMsg := data.NewMessage(data.READ, "store", "BTC", nil)
				var readYields []float64

				for ptr := range op.Next(readMsg.Next(nil)) {
					prim := *(*core.Primitive)(ptr)

					for entryPtr := range prim.Next(nil) {
						readYields = append(readYields, *(*float64)(entryPtr))
					}
				}

				So(len(readYields), ShouldEqual, 1)
				So(readYields[0], ShouldEqual, 42.0)
			})
		})
	})
}
