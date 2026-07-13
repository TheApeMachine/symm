package system_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func TestBooterReady(t *testing.T) {
	Convey("Given a booter with a preflight stage and a warmup stage", t, func() {
		api := tests.NewMockReporter(types.READY)

		instrument := tests.NewMockReporter(types.INITIALIZING)
		instrument.SetReadyOnInitialize(true)

		signal := tests.NewMockReporter(types.INITIALIZING)
		signal.SetReadyOnInitialize(true)

		ready := tests.NewMockReporter(types.INITIALIZING)
		ready.SetReadyOnInitialize(true)

		booter := system.NewBooter(context.Background(), nil)
		booter.AddStages(
			system.NewStage(system.StagePreflight, api, instrument),
			system.NewStage(system.StageWarmup, signal),
			system.NewStage(system.StageReady, ready),
		)

		// AddStages already resolved every reporter to READY through its own
		// Initialize. Reset them so each scenario below can drive its own
		// partial-boot state without racing a background Initialize call.
		instrument.SetStatus(types.INITIALIZING)
		signal.SetStatus(types.INITIALIZING)
		ready.SetStatus(types.INITIALIZING)

		Convey("When preflight is not fully ready", func() {
			Convey("Then Ready reports false for preflight", func() {
				So(booter.Ready(system.StagePreflight), ShouldBeFalse)
			})

			Convey("And Ready for warmup is unaffected by preflight's state", func() {
				signal.SetStatus(types.READY)
				So(booter.Ready(system.StageWarmup), ShouldBeTrue)
			})
		})

		Convey("When every preflight reporter becomes READY", func() {
			instrument.SetStatus(types.READY)

			Convey("Then Ready reports true for preflight only", func() {
				So(booter.Ready(system.StagePreflight), ShouldBeTrue)
				So(booter.Ready(system.StageWarmup), ShouldBeFalse)
			})
		})

		Convey("When the ready stage has not reported READY yet", func() {
			Convey("Then Ready reports false for ready", func() {
				So(booter.Ready(system.StageReady), ShouldBeFalse)
			})
		})

		Convey("When asked about a stage that was never registered", func() {
			Convey("Then Ready reports false rather than panicking", func() {
				So(booter.Ready(system.StageType(99)), ShouldBeFalse)
			})
		})

		Convey("When preflight is still initializing", func() {
			Convey("Then CurrentPhase reports scan", func() {
				So(booter.CurrentPhase(), ShouldEqual, "scan")
			})
		})

		Convey("When preflight is ready and warmup is still initializing", func() {
			instrument.SetStatus(types.READY)

			Convey("Then CurrentPhase reports evaluate", func() {
				So(booter.CurrentPhase(), ShouldEqual, "evaluate")
			})
		})

		Convey("When every stage is ready", func() {
			instrument.SetStatus(types.READY)
			signal.SetStatus(types.READY)
			ready.SetStatus(types.READY)

			Convey("Then CurrentPhase reports commit", func() {
				So(booter.CurrentPhase(), ShouldEqual, "commit")
			})
		})
	})
}
