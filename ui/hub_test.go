package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHubDashboardSnapshot(t *testing.T) {
	Convey("Given cached dashboard frames", t, func() {
		hub := &Hub{}
		status := map[string]any{"event": "status", "equity": 200.0}

		hub.cacheSnapshot(map[string]any{"event": "scoreboard", "rows": 3})
		hub.cacheSnapshot(status)
		hub.cacheSnapshot(map[string]any{"event": "candle_bar", "close": 1.0})
		hub.cacheSnapshot(map[string]any{"event": "decision_trace", "tick": 9})
		hub.cacheSnapshot(map[string]any{"event": "engine_pulse", "tick": 9})
		status["equity"] = 0.0

		frames := hub.dashboardSnapshot()

		Convey("It should replay only dashboard snapshot frames in publish order", func() {
			So(len(frames), ShouldEqual, 4)
			So(frames[0]["event"], ShouldEqual, "engine_pulse")
			So(frames[1]["event"], ShouldEqual, "decision_trace")
			So(frames[2]["event"], ShouldEqual, "scoreboard")
			So(frames[3]["event"], ShouldEqual, "status")
			So(frames[3]["equity"], ShouldEqual, 200.0)
		})

		Convey("It should return copied frames", func() {
			frames[3]["equity"] = 0.0

			nextFrames := hub.dashboardSnapshot()

			So(nextFrames[3]["equity"], ShouldEqual, 200.0)
		})
	})
}
