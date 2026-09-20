package arithmetic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
)

func TestMultiplyPrimitive(t *testing.T) {
	Convey("Given a native Multiply Cap'n Proto server", t, func() {
		server := arithmetic.NewMultiply()
		So(server, ShouldNotBeNil)

		client := arithmetic.Multiply_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When invoking Write with a and b", func() {
			ctx := context.Background()

			err := client.Write(ctx, func(params arithmetic.Multiply_write_Params) error {
				params.SetA(2.5)
				params.SetB(4.0)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 10.0)

			Convey("When performing a second evaluation, previous state is reset", func() {
				err = client.Write(ctx, func(params arithmetic.Multiply_write_Params) error {
					params.SetA(3.0)
					params.SetB(7.0)
					return nil
				})
				So(err, ShouldBeNil)

				err = client.WaitStreaming()
				So(err, ShouldBeNil)

				secondFuture, secondRelease := client.Done(ctx, nil)
				defer secondRelease()

				secondResults, err := secondFuture.Struct()
				So(err, ShouldBeNil)
				So(secondResults.Out(), ShouldEqual, 21.0)
			})
		})
	})
}
