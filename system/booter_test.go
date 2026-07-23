package system_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

func TestBooterReadyBeforeStages(t *testing.T) {
	Convey("Given a booter with no stages registered yet", t, func() {
		booter := system.NewBooter(context.Background(), nil)

		Convey("Then Ready is false instead of panicking", func() {
			So(booter.Ready(system.StageWarmup), ShouldBeFalse)
		})
	})
}

func TestBooterReady(t *testing.T) {
	Convey("Given a booter with a preflight stage and a warmup stage", t, func() {
		api := newMockReporter(types.READY)

		instrument := newMockReporter(types.INITIALIZING)
		instrument.SetReadyOnInitialize(true)

		signal := newMockReporter(types.INITIALIZING)
		signal.SetReadyOnInitialize(true)

		ready := newMockReporter(types.INITIALIZING)
		ready.SetReadyOnInitialize(true)

		booter := system.NewBooter(context.Background(), nil)
		booter.AddStages(
			system.NewStage(system.StagePreflight, api, instrument),
			system.NewStage(system.StageWarmup, signal),
			system.NewStage(system.StageReady, ready),
		)

		So(booter.Start(), ShouldBeNil)

		// Start already resolved every reporter to READY through each
		// stage's own Initialize. Reset them so each scenario below can
		// drive its own partial-boot state.
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
	})
}

func TestBooterError(t *testing.T) {
	Convey("Given a booter with a reporter that fails to initialize", t, func() {
		failing := newMockReporter(types.INITIALIZING)
		failing.SetInitializeError(errors.New("boom"))

		booter := system.NewBooter(context.Background(), nil)
		booter.AddStages(system.NewStage(system.StagePreflight, failing))

		Convey("When Start runs that stage", func() {
			err := booter.Start()

			Convey("Then Start reports the failure and Error is true", func() {
				So(err, ShouldNotBeNil)
				So(booter.Error(), ShouldBeTrue)
			})
		})
	})
}
