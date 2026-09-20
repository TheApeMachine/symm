package arithmetic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
)

func TestDividePrimitive(t *testing.T) {
	Convey("Given a native Divide Cap'n Proto server", t, func() {
		server := arithmetic.NewDivide()
		So(server, ShouldNotBeNil)

		client := arithmetic.Divide_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When invoking Write with a valid divisor", func() {
			ctx := context.Background()

			err := client.Write(ctx, func(params arithmetic.Divide_write_Params) error {
				params.SetA(15.0)
				params.SetB(3.0)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 5.0)

			Convey("When dividing by zero, an error is returned on streaming flush", func() {
				err = client.Write(ctx, func(params arithmetic.Divide_write_Params) error {
					params.SetA(10.0)
					params.SetB(0.0)
					return nil
				})
				So(err, ShouldBeNil)

				err = client.WaitStreaming()
				So(err, ShouldNotBeNil)
			})

			Convey("When performing a second valid evaluation, state is reset", func() {
				err = client.Write(ctx, func(params arithmetic.Divide_write_Params) error {
					params.SetA(40.0)
					params.SetB(8.0)
					return nil
				})
				So(err, ShouldBeNil)

				err = client.WaitStreaming()
				So(err, ShouldBeNil)

				secondFuture, secondRelease := client.Done(ctx, nil)
				defer secondRelease()

				secondResults, err := secondFuture.Struct()
				So(err, ShouldBeNil)
				So(secondResults.Out(), ShouldEqual, 5.0)
			})
		})
	})
}
