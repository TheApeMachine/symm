package controlflow

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestDelayServer(t *testing.T) {
	Convey("Given a DelayServer with 50ms pacing", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := NewDelay(ctx)
		So(server, ShouldNotBeNil)
		So(server.Status(), ShouldEqual, runtime.READY)

		capDelay := Delay_ServerToClient(server)
		So(capDelay.IsValid(), ShouldBeTrue)

		Convey("First write emits immediately", func() {
			err := capDelay.Write(ctx, func(params Delay_write_Params) error {
				params.SetMillis(50)
				return params.SetData([]byte(`"batch-1"`))
			})
			So(err, ShouldBeNil)

			future, release := capDelay.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Ready(), ShouldBeTrue)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, `"batch-1"`)

			Convey("Immediate subsequent write is gated (not ready)", func() {
				err := capDelay.Write(ctx, func(params Delay_write_Params) error {
					return params.SetData([]byte(`"batch-2"`))
				})
				So(err, ShouldBeNil)

				future2, release2 := capDelay.Done(ctx, nil)
				defer release2()

				results2, err := future2.Struct()
				So(err, ShouldBeNil)
				So(results2.Ready(), ShouldBeFalse)

				Convey("After waiting 55ms, write is allowed through", func() {
					time.Sleep(55 * time.Millisecond)

					err := capDelay.Write(ctx, func(params Delay_write_Params) error {
						return params.SetData([]byte(`"batch-2"`))
					})
					So(err, ShouldBeNil)

					future3, release3 := capDelay.Done(ctx, nil)
					defer release3()

					results3, err := future3.Struct()
					So(err, ShouldBeNil)
					So(results3.Ready(), ShouldBeTrue)

					out3, err := results3.Out()
					So(err, ShouldBeNil)
					So(string(out3), ShouldEqual, `"batch-2"`)
				})
			})
		})
	})
}
