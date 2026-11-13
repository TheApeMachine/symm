package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewUI(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewUI is called", func() {
			ui := NewUI()

			Convey("dashboard listen defaults land from viper", func() {
				So(ui.Addr, ShouldEqual, "127.0.0.1:8765")
				So(ui.WebSocket.MaxMessageBytes, ShouldEqual, 4194304)
				So(ui.WebSocket.QueueFrames, ShouldEqual, 256)
			})
		})
	})
}
