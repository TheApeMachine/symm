package cmd

import (
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEngineWorkerCount(t *testing.T) {
	Convey("Given no engine worker override", t, func() {
		t.Setenv(engineWorkersEnv, "")

		workers, err := engineWorkerCount()

		Convey("It should preserve the default engine worker budget", func() {
			So(err, ShouldBeNil)
			So(workers, ShouldEqual, runtime.NumCPU()*4)
		})
	})

	Convey("Given an engine worker override", t, func() {
		t.Setenv(engineWorkersEnv, "3")

		workers, err := engineWorkerCount()

		Convey("It should use the requested worker budget", func() {
			So(err, ShouldBeNil)
			So(workers, ShouldEqual, 3)
		})
	})
}
