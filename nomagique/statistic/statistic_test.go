package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestStatisticPrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given statistical primitives", t, func() {
		Convey("Mean evaluates cumulative mean and resets on Done", func() {
			server := statistic.NewMean()
			client := statistic.Mean_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params statistic.Mean_write_Params) error {
				params.SetValue(10.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 10.0)

			Convey("When performing a second evaluation", func() {
				err = client.Write(ctx, func(params statistic.Mean_write_Params) error {
					params.SetValue(20.0)
					return nil
				})
				So(err, ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)

				secondFuture, secondRelease := client.Done(ctx, nil)
				defer secondRelease()

				secondResults, err := secondFuture.Struct()
				So(err, ShouldBeNil)
				So(secondResults.Out(), ShouldEqual, 15.0)
			})
		})

		Convey("Variance evaluates sample variance", func() {
			server := statistic.NewVariance()
			client := statistic.Variance_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params statistic.Variance_write_Params) error {
				params.SetValue(2.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 0.0)

			err = client.Write(ctx, func(params statistic.Variance_write_Params) error {
				params.SetValue(4.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future2, release2 := client.Done(ctx, nil)
			defer release2()

			results2, err := future2.Struct()
			So(err, ShouldBeNil)
			So(results2.Out(), ShouldEqual, 2.0)
		})

		Convey("Threshold classifies values relative to band", func() {
			server := statistic.NewThreshold(0.2, 0.0, -1.0, 1.0)
			client := statistic.Threshold_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params statistic.Threshold_write_Params) error {
				params.SetValue(0.1)
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
	})
}
