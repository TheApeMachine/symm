package controlflow

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestConditionServer(t *testing.T) {
	Convey("Given a ConditionServer", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := NewCondition(ctx)
		So(server, ShouldNotBeNil)
		So(server.Status(), ShouldEqual, runtime.READY)

		capCond := Condition_ServerToClient(server)
		So(capCond.IsValid(), ShouldBeTrue)

		Convey("When status is Status_ready", func() {
			err := capCond.Write(ctx, func(params Condition_write_Params) error {
				params.SetStatus(runtime.Status_ready)
				return nil
			})
			So(err, ShouldBeNil)

			future, release := capCond.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Ready(), ShouldBeTrue)
			So(results.Busy(), ShouldBeFalse)
			So(results.Waiting(), ShouldBeFalse)
			So(results.Error(), ShouldBeFalse)
			So(results.Done(), ShouldBeFalse)
		})

		Convey("When status is Status_busy", func() {
			err := capCond.Write(ctx, func(params Condition_write_Params) error {
				params.SetStatus(runtime.Status_busy)
				return nil
			})
			So(err, ShouldBeNil)

			future, release := capCond.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Ready(), ShouldBeFalse)
			So(results.Busy(), ShouldBeTrue)
		})

		Convey("When test is true", func() {
			err := capCond.Write(ctx, func(params Condition_write_Params) error {
				params.SetTest(true)
				return nil
			})
			So(err, ShouldBeNil)

			future, release := capCond.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Ready(), ShouldBeTrue)
		})
	})
}
