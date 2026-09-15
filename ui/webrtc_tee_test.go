package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestWebRTCTeePushReadiness(t *testing.T) {
	Convey("The WebRTC tee drops input until startup activates it", t, func() {
		tee := NewWebRTCTee("webrtc-readiness", 4)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		measurement := &data.Measurement[float64]{Label: "BTC/USD"}
		tee.Push(measurement)
		So(tee.ring.Len(), ShouldEqual, 0)

		tee.Transition(runtime.READY)
		tee.Push(measurement)
		So((*data.Measurement[float64])(tee.Next()), ShouldEqual, measurement)
		So(tee.ring.Len(), ShouldEqual, 0)
	})
}
