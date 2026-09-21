package learning_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning"
)

func TestLearningPrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given learning target primitives", t, func() {
		Convey("BinaryTarget returns 1 when current > past and resets on Done", func() {
			server := learning.NewBinaryTarget()
			client := learning.BinaryTarget_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params learning.BinaryTarget_write_Params) error {
				params.SetPast(10.0)
				params.SetCurrent(20.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 1.0)
		})

		Convey("DeltaTarget returns current - past", func() {
			server := learning.NewDeltaTarget()
			client := learning.DeltaTarget_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params learning.DeltaTarget_write_Params) error {
				params.SetPast(15.0)
				params.SetCurrent(25.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 10.0)
		})

		Convey("RatioTarget returns current/past - 1", func() {
			server := learning.NewRatioTarget()
			client := learning.RatioTarget_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params learning.RatioTarget_write_Params) error {
				params.SetPast(100.0)
				params.SetCurrent(105.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldAlmostEqual, 0.05, 1e-9)
		})

		Convey("Forecast computes running moments", func() {
			server := learning.NewForecast()
			client := learning.Forecast_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params learning.Forecast_write_Params) error {
				params.SetValue(10.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Mean(), ShouldEqual, 10.0)
		})
	})
}
