package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

type stubReporter struct {
	status types.Status
}

func (reporter *stubReporter) Status() types.Status {
	return reporter.status
}

func TestStageStatus(t *testing.T) {
	Convey("Given a stage with two reporters that have not reported READY yet", t, func() {
		first := &stubReporter{status: types.INITIALIZING}
		second := &stubReporter{status: types.INITIALIZING}
		stage := NewStage(StagePreflight, first, second)

		Convey("Then the stage reports INITIALIZING", func() {
			So(stage.Status(), ShouldEqual, types.INITIALIZING)
		})

		Convey("When every reporter becomes READY", func() {
			first.status = types.READY
			second.status = types.READY

			Convey("Then the stage reports READY", func() {
				So(stage.Status(), ShouldEqual, types.READY)
			})

			Convey("And when a reporter later drops out of READY", func() {
				stage.Status()
				second.status = types.INITIALIZING

				Convey("Then the stage reports PENDING, not INITIALIZING", func() {
					So(stage.Status(), ShouldEqual, types.PENDING)
				})
			})
		})
	})
}
