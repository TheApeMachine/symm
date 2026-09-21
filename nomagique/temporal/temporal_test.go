package temporal_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
)

func TestTemporalPrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given temporal primitives", t, func() {
		Convey("LogReturns calculates logarithmic returns", func() {
			server := temporal.NewLogReturns()
			client := temporal.LogReturns_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params temporal.LogReturns_write_Params) error {
				params.SetValue(100.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 0.0)

			err = client.Write(ctx, func(params temporal.LogReturns_write_Params) error {
				params.SetValue(110.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future2, release2 := client.Done(ctx, nil)
			defer release2()

			results2, err := future2.Struct()
			So(err, ShouldBeNil)
			So(results2.Out(), ShouldAlmostEqual, math.Log(1.1))
		})

		Convey("Velocity calculates rate of change over time", func() {
			server := temporal.NewVelocity()
			client := temporal.Velocity_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params temporal.Velocity_write_Params) error {
				params.SetVal(10.0)
				params.SetTs(1.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 0.0)

			err = client.Write(ctx, func(params temporal.Velocity_write_Params) error {
				params.SetVal(20.0)
				params.SetTs(3.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future2, release2 := client.Done(ctx, nil)
			defer release2()

			results2, err := future2.Struct()
			So(err, ShouldBeNil)
			So(results2.Out(), ShouldEqual, 5.0)
		})
	})
}
