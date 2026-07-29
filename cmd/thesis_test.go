package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestRestoreThesis(t *testing.T) {
	Convey("Given an active thesis and optional recovery state", t, func() {
		channel := make(chan []byte, 1)
		active := types.NewThesis(nil)
		active.Tick = 7

		Convey("It should retain the active thesis when recovery state is malformed", func() {
			restored := restoreThesis(
				active, channel, []byte(`{"tick":`), "invalid optional thesis", nil,
			)

			So(restored, ShouldEqual, active)
			So(restored.Tick, ShouldEqual, 7)
		})

		Convey("It should replace the active thesis when recovery state is valid", func() {
			restored := restoreThesis(
				active, channel, []byte(`{"tick":11}`), "invalid optional thesis", nil,
			)

			So(restored, ShouldNotEqual, active)
			So(restored.Tick, ShouldEqual, 11)
		})
	})
}
