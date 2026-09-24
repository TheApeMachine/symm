package geometry_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestInversion(t *testing.T) {
	ctx := context.Background()

	Convey("Given signed relationship strengths", t, func() {
		client := geometry.Inversion_ServerToClient(geometry.NewInversion())
		defer client.Release()

		So(client.Write(ctx, func(params geometry.Inversion_write_Params) error {
			strength, err := params.NewStrength(3)

			if err != nil {
				return err
			}

			strength.Set(0, 1)
			strength.Set(1, 0)
			strength.Set(2, -1)
			return nil
		}), ShouldBeNil)
		So(client.WaitStreaming(), ShouldBeNil)

		future, release := client.Done(ctx, nil)
		defer release()

		results, err := future.Struct()
		So(err, ShouldBeNil)

		Convey("Sympathy asks for less than a cell, none for one cell, repulsion for more", func() {
			distance, err := results.Distance()
			So(err, ShouldBeNil)
			So(distance.At(0), ShouldEqual, 0.5)
			So(distance.At(1), ShouldEqual, 1)
			So(distance.At(2), ShouldEqual, 2)
		})
	})
}
