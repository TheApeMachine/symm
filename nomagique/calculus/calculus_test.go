package calculus_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
)

func TestCalculusPrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given unary calculus primitives", t, func() {
		Convey("Absolute evaluates correctly", func() {
			server := calculus.NewAbsolute()
			client := calculus.Absolute_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params calculus.Absolute_write_Params) error {
				params.SetIn(-42.5)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 42.5)
		})

		Convey("Tanh evaluates correctly", func() {
			server := calculus.NewTanh()
			client := calculus.Tanh_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params calculus.Tanh_write_Params) error {
				params.SetIn(0.5)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, math.Tanh(0.5))
		})

		Convey("Square evaluates correctly", func() {
			server := calculus.NewSquare()
			client := calculus.Square_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params calculus.Square_write_Params) error {
				params.SetIn(3.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 9.0)
		})

		Convey("Sqrt evaluates correctly and guards negative values", func() {
			server := calculus.NewSqrt()
			client := calculus.Sqrt_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params calculus.Sqrt_write_Params) error {
				params.SetIn(16.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 4.0)

			err = client.Write(ctx, func(params calculus.Sqrt_write_Params) error {
				params.SetIn(-1.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})

		Convey("Negate evaluates correctly", func() {
			server := calculus.NewNegate()
			client := calculus.Negate_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params calculus.Negate_write_Params) error {
				params.SetIn(5.5)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, -5.5)
		})

		Convey("Reciprocal evaluates correctly and guards zero", func() {
			server := calculus.NewReciprocal()
			client := calculus.Reciprocal_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params calculus.Reciprocal_write_Params) error {
				params.SetIn(4.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 0.25)

			err = client.Write(ctx, func(params calculus.Reciprocal_write_Params) error {
				params.SetIn(0.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})

	Convey("Given binary and polyadic calculus primitives", t, func() {
		Convey("Maximum evaluates correctly", func() {
			server := calculus.NewMaximum()
			client := calculus.Maximum_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params calculus.Maximum_write_Params) error {
				params.SetA(10.0)
				params.SetB(20.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 20.0)
		})

		Convey("Minimum evaluates correctly", func() {
			server := calculus.NewMinimum()
			client := calculus.Minimum_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params calculus.Minimum_write_Params) error {
				params.SetA(10.0)
				params.SetB(20.0)
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

		Convey("Bound clamps values between min and max", func() {
			server := calculus.NewBound()
			client := calculus.Bound_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params calculus.Bound_write_Params) error {
				params.SetIn(50.0)
				params.SetMin(0.0)
				params.SetMax(10.0)
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

		Convey("RelativeChange evaluates change relative to previous", func() {
			server := calculus.NewRelativeChange()
			client := calculus.RelativeChange_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params calculus.RelativeChange_write_Params) error {
				params.SetIn(110.0)
				params.SetPrev(100.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldAlmostEqual, 0.1)
		})
	})
}
