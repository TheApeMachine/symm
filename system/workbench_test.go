package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewWorkbench(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewWorkbench is called", func() {
			wb := NewWorkbench()

			Convey("workbench defaults land from viper", func() {
				So(wb.Addr, ShouldEqual, ":8081")
				So(wb.URL, ShouldEqual, "http://127.0.0.1:8081/workbench/query")
				So(wb.MemoryLimit, ShouldEqual, "16GB")
				So(wb.MaxTempDirectorySize, ShouldEqual, "64GB")
				So(wb.Threads, ShouldEqual, 4)
			})
		})
	})
}
