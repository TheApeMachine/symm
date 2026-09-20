package arithmetic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
)

func TestSubtractPrimitive(t *testing.T) {
	Convey("Given a native Subtract Cap'n Proto server", t, func() {
		server := arithmetic.NewSubtract()
		So(server, ShouldNotBeNil)

		client := arithmetic.Subtract_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When invoking Write with a and b", func() {
			ctx := context.Background()

			err := client.Write(ctx, func(params arithmetic.Subtract_write_Params) error {
				params.SetA(10.5)
				params.SetB(3.5)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 7.0)

			Convey("When performing a second evaluation, previous state is reset", func() {
				err = client.Write(ctx, func(params arithmetic.Subtract_write_Params) error {
					params.SetA(20.0)
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
				So(secondResults.Out(), ShouldEqual, 12.0)
			})
		})
	})
}
