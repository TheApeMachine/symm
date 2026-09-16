package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

func TestWebRTCTeePushReadiness(t *testing.T) {
	Convey("The WebRTC tee drops input until startup activates it", t, func() {
		tee := NewWebRTCTee("webrtc-readiness", 4)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		artifact := &types.ManifoldState{Version: 1}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", Artifact: artifact}
		tee.Push(measurement)
		So(tee.ring.Len(), ShouldEqual, 0)

		tee.Transition(runtime.READY)
		tee.Push(measurement)
		So(*(*any)(tee.Next()), ShouldEqual, artifact)
		So(tee.ring.Len(), ShouldEqual, 0)
	})
}

func BenchmarkWebRTCTeePush(b *testing.B) {
	tee := NewWebRTCTee("benchmark", 4)
	tee.Transition(runtime.READY)
	artifact := &types.ManifoldState{Version: 1}
	measurement := &data.Measurement[float64]{Artifact: artifact}
	
	for b.Loop() {
		tee.Push(measurement)
		if *(*any)(tee.Next()) != artifact {
			b.Fatal("artifact changed")
		}
	}
	b.StopTimer()
	if err := tee.Close(); err != nil {
		b.Fatal(err)
	}
}
