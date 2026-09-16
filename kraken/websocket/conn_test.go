package websocket

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestNewAPI(t *testing.T) {
	Convey("Wrapping transports preserves their connection states", t, func() {
		for _, stage := range []runtime.Stage{runtime.BUSY, runtime.WAITING, runtime.ERROR} {
			public := &Live{System: runtime.NewSystem(t.Context(), "public")}
			private := &Live{System: runtime.NewSystem(t.Context(), "private")}
			futures := &FuturesLive{System: runtime.NewSystem(t.Context(), "futures")}
			public.Transition(stage)
			private.Transition(stage)
			futures.Transition(stage)

			api := NewAPI(t.Context(), public, private, futures)
			So(api.Status(), ShouldEqual, runtime.WAITING)
			So(public.Status(), ShouldEqual, stage)
			So(private.Status(), ShouldEqual, stage)
			So(futures.Status(), ShouldEqual, stage)

			if stage == runtime.BUSY {
				Convey("API activation preserves each transport until explicitly activated", func() {
					api.Transition(runtime.READY)
					So(api.Status(), ShouldEqual, runtime.READY)
					So(public.Status(), ShouldEqual, runtime.BUSY)
					So(private.Status(), ShouldEqual, runtime.BUSY)
					So(futures.Status(), ShouldEqual, runtime.BUSY)
					public.Transition(runtime.READY)
					private.Transition(runtime.READY)
					futures.Transition(runtime.READY)
					So(public.Status(), ShouldEqual, runtime.READY)
					So(private.Status(), ShouldEqual, runtime.READY)
					So(futures.Status(), ShouldEqual, runtime.READY)
				})
			}
		}
	})
}
