package calculus_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
)

func TestAtanhPrimitive(t *testing.T) {
	Convey("Given a native Atanh Cap'n Proto server", t, func() {
		server := calculus.NewAtanh()
		So(server, ShouldNotBeNil)

		client := calculus.Atanh_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When invoking Write with a float64 value", func() {
			input := 0.5
			ctx := context.Background()

			err := client.Write(ctx, func(params calculus.Atanh_write_Params) error {
				params.SetValue(input)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, math.Atanh(input))

			Convey("When performing a second evaluation, previous state is reset", func() {
				secondInput := 0.25

				err = client.Write(ctx, func(params calculus.Atanh_write_Params) error {
					params.SetValue(secondInput)
					return nil
				})
				So(err, ShouldBeNil)

				err = client.WaitStreaming()
				So(err, ShouldBeNil)

				secondFuture, secondRelease := client.Done(ctx, nil)
				defer secondRelease()

				secondResults, err := secondFuture.Struct()
				So(err, ShouldBeNil)
				So(secondResults.Out(), ShouldEqual, math.Atanh(secondInput))
			})
		})
	})
}
