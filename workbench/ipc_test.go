package workbench

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAsInt64(t *testing.T) {
	Convey("Given DuckDB unsigned integers", t, func() {
		Convey("It should keep uint, uint8, uint16, uint32, and uint64", func() {
			So(asInt64(uint(42)), ShouldEqual, 42)
			So(asInt64(uint8(7)), ShouldEqual, 7)
			So(asInt64(uint16(1000)), ShouldEqual, 1000)
			So(asInt64(uint32(100000)), ShouldEqual, 100000)
			So(asInt64(uint64(99)), ShouldEqual, 99)
			So(asInt64(uint64(math.MaxUint64)), ShouldEqual, math.MaxInt64)
		})

		Convey("It should keep signed values", func() {
			So(asInt64(int32(-3)), ShouldEqual, -3)
			So(asInt64(int64(11)), ShouldEqual, 11)
		})
	})
}
