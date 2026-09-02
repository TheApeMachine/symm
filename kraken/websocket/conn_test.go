package websocket

import (
	"context"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestAPIRun(t *testing.T) {
	Convey("Given required spot sessions bound to the API", t, func() {
		publicCtx, publicCancel := context.WithCancel(t.Context())
		privateCtx, privateCancel := context.WithCancel(t.Context())
		defer publicCancel()
		defer privateCancel()
		public := &Live{
			ctx:    publicCtx,
			cancel: publicCancel,
			status: runtime.NewStatus(),
		}
		private := &Live{
			ctx:    privateCtx,
			cancel: privateCancel,
			status: runtime.NewStatus(),
		}
		api := NewAPI(t.Context(), public, private)
		expected := errors.New("level3 checksum mismatch")

		Convey("A child failure should persist before Run and halt the API", func() {
			public.fail(expected)

			So(errors.Is(api.Error(), expected), ShouldBeTrue)
			So(errors.Is(api.Run(), expected), ShouldBeTrue)
			So(api.Status(), ShouldEqual, runtime.ERROR)
			So(api.ctx.Err(), ShouldEqual, context.Canceled)
		})
	})

	Convey("Given a futures session that failed before attachment", t, func() {
		publicCtx, publicCancel := context.WithCancel(t.Context())
		privateCtx, privateCancel := context.WithCancel(t.Context())
		futuresCtx, futuresCancel := context.WithCancel(t.Context())
		defer publicCancel()
		defer privateCancel()
		defer futuresCancel()
		public := &Live{
			ctx: publicCtx, cancel: publicCancel,
			status:  runtime.NewStatus().Transition(runtime.BUSY),
			ingress: readyTestIngress("ticker", "trade"),
		}
		private := &Live{
			ctx: privateCtx, cancel: privateCancel,
			status:  runtime.NewStatus().Transition(runtime.BUSY),
			ingress: readyTestIngress("level3", "executions"),
		}
		futures := &FuturesLive{
			ctx: futuresCtx, cancel: futuresCancel,
			status:  runtime.NewStatus().Transition(runtime.BUSY),
			ingress: readyTestIngress("ticker", "trade"),
		}
		api := NewAPI(t.Context(), public, private)
		expected := errors.New("futures disconnected")
		futures.fail(expected)

		Convey("Attaching it should replay the failure to the API", func() {
			api.SetFutures(futures)

			So(errors.Is(api.Error(), expected), ShouldBeTrue)
			So(errors.Is(api.Run(), expected), ShouldBeTrue)
		})
	})
}

func TestAPIMarkReady(t *testing.T) {
	Convey("Given spot sessions and a required futures session", t, func() {
		publicCtx, publicCancel := context.WithCancel(t.Context())
		privateCtx, privateCancel := context.WithCancel(t.Context())
		futuresCtx, futuresCancel := context.WithCancel(t.Context())
		defer publicCancel()
		defer privateCancel()
		defer futuresCancel()
		public := &Live{
			ctx: publicCtx, cancel: publicCancel,
			status:  runtime.NewStatus().Transition(runtime.BUSY),
			ingress: readyTestIngress("ticker", "trade"),
		}
		private := &Live{
			ctx: privateCtx, cancel: privateCancel,
			status:  runtime.NewStatus().Transition(runtime.BUSY),
			ingress: readyTestIngress("level3", "executions"),
		}
		futures := &FuturesLive{
			ctx: futuresCtx, cancel: futuresCancel,
			status:  runtime.NewStatus().Transition(runtime.BUSY),
			ingress: readyTestIngress("ticker", "trade"),
		}
		api := NewAPI(t.Context(), public, private)
		api.SetFutures(futures)
		public.MarkReady()
		private.MarkReady()

		Convey("Futures should remain part of the readiness barrier", func() {
			So(api.Status(), ShouldEqual, runtime.INIT)

			api.MarkReady()

			So(public.Status(), ShouldEqual, runtime.READY)
			So(private.Status(), ShouldEqual, runtime.READY)
			So(futures.Status(), ShouldEqual, runtime.READY)
			So(api.Status(), ShouldEqual, runtime.READY)
		})
	})
}
