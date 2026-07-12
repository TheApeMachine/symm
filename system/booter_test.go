package system

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestBooterReady(t *testing.T) {
	Convey("Given a booter with a preflight stage and a warmup stage", t, func() {
		api := &stubReporter{status: types.READY}
		instrument := &stubReporter{status: types.INITIALIZING}
		signal := &stubReporter{status: types.INITIALIZING}
		ready := &stubReporter{status: types.INITIALIZING}

		booter := NewBooter(context.Background(), nil)
		booter.AddStages(
			NewStage(StagePreflight, api, instrument),
			NewStage(StageWarmup, signal),
			NewStage(StageReady, ready),
		)

		Convey("When preflight is not fully ready", func() {
			Convey("Then Ready reports false for preflight", func() {
				So(booter.Ready(StagePreflight), ShouldBeFalse)
			})

			Convey("And Ready for warmup is unaffected by preflight's state", func() {
				signal.status = types.READY
				So(booter.Ready(StageWarmup), ShouldBeTrue)
			})
		})

		Convey("When every preflight reporter becomes READY", func() {
			instrument.status = types.READY

			Convey("Then Ready reports true for preflight only", func() {
				So(booter.Ready(StagePreflight), ShouldBeTrue)
				So(booter.Ready(StageWarmup), ShouldBeFalse)
			})
		})

		Convey("When the ready stage has not reported READY yet", func() {
			Convey("Then Ready reports false for ready", func() {
				So(booter.Ready(StageReady), ShouldBeFalse)
			})
		})

		Convey("When asked about a stage that was never registered", func() {
			Convey("Then Ready reports false rather than panicking", func() {
				So(booter.Ready(StageType(99)), ShouldBeFalse)
			})
		})

		Convey("When preflight is still initializing", func() {
			Convey("Then CurrentPhase reports scan", func() {
				So(booter.CurrentPhase(), ShouldEqual, "scan")
			})
		})

		Convey("When preflight is ready and warmup is still initializing", func() {
			instrument.status = types.READY

			Convey("Then CurrentPhase reports evaluate", func() {
				So(booter.CurrentPhase(), ShouldEqual, "evaluate")
			})
		})

		Convey("When every stage is ready", func() {
			instrument.status = types.READY
			signal.status = types.READY
			ready.status = types.READY

			Convey("Then CurrentPhase reports commit", func() {
				So(booter.CurrentPhase(), ShouldEqual, "commit")
			})
		})
	})
}
