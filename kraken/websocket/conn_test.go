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
				Convey("Explicit activation still opens all connected transports", func() {
					api.Transition(runtime.READY)
					So(api.Status(), ShouldEqual, runtime.READY)
					So(public.Status(), ShouldEqual, runtime.READY)
					So(private.Status(), ShouldEqual, runtime.READY)
					So(futures.Status(), ShouldEqual, runtime.READY)
				})
			}
		}
	})
}
