package controlflow

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestOnceServer(t *testing.T) {
	Convey("Given a OnceServer", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := NewOnce(ctx)
		So(server, ShouldNotBeNil)
		So(server.Status(), ShouldEqual, runtime.READY)

		capOnce := Once_ServerToClient(server)
		So(capOnce.IsValid(), ShouldBeTrue)

		payload := []byte(`{"method":"instrument"}`)

		Convey("When providing payload without trigger", func() {
			err := capOnce.Write(ctx, func(params Once_write_Params) error {
				return params.SetThrough(payload)
			})
			So(err, ShouldBeNil)

			future, release := capOnce.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Fired(), ShouldBeFalse)
			So(results.HasOut(), ShouldBeFalse)
		})

		Convey("When trigger is true, it passes payload once", func() {
			err := capOnce.Write(ctx, func(params Once_write_Params) error {
				if err := params.SetThrough(payload); err != nil {
					return err
				}
				params.SetTrigger(true)
				return nil
			})
			So(err, ShouldBeNil)

			future, release := capOnce.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Fired(), ShouldBeTrue)
			So(results.HasOut(), ShouldBeTrue)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, string(payload))

			Convey("Subsequent ticks do not fire again", func() {
				err := capOnce.Write(ctx, func(params Once_write_Params) error {
					params.SetTrigger(true)
					return nil
				})
				So(err, ShouldBeNil)

				future2, release2 := capOnce.Done(ctx, nil)
				defer release2()

				results2, err := future2.Struct()
				So(err, ShouldBeNil)
				So(results2.Fired(), ShouldBeTrue)
				So(results2.HasOut(), ShouldBeFalse)
			})

			Convey("Reset allows it to fire again", func() {
				err := capOnce.Write(ctx, func(params Once_write_Params) error {
					params.SetReset(true)
					params.SetTrigger(true)
					return nil
				})
				So(err, ShouldBeNil)

				future3, release3 := capOnce.Done(ctx, nil)
				defer release3()

				results3, err := future3.Struct()
				So(err, ShouldBeNil)
				So(results3.Fired(), ShouldBeTrue)
				So(results3.HasOut(), ShouldBeTrue)
			})
		})
	})
}
