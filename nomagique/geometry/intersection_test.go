package geometry_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestIntersectionPrimitive(t *testing.T) {
	ctx := context.Background()

	Convey("Given an Intersection Cap'n Proto server", t, func() {
		server := geometry.NewIntersection()
		client := geometry.Intersection_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When intervals overlap", func() {
			err := client.Write(ctx, func(params geometry.Intersection_write_Params) error {
				params.SetLeftStart(10)
				params.SetLeftEnd(30)
				params.SetRightStart(20)
				params.SetRightEnd(40)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldBeTrue)
		})

		Convey("When intervals do not overlap", func() {
			err := client.Write(ctx, func(params geometry.Intersection_write_Params) error {
				params.SetLeftStart(10)
				params.SetLeftEnd(20)
				params.SetRightStart(30)
				params.SetRightEnd(40)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldBeFalse)
		})
	})
}
