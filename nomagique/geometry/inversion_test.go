package geometry_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestInversion(t *testing.T) {
	ctx := context.Background()

	Convey("Given signed relationship strengths", t, func() {
		client := geometry.Inversion_ServerToClient(geometry.NewInversion())
		defer client.Release()

		invert := func(strengths ...float64) []float64 {
			So(client.Write(ctx, func(params geometry.Inversion_write_Params) error {
				list, err := params.NewStrength(int32(len(strengths)))

				if err != nil {
					return err
				}

				for index, value := range strengths {
					list.Set(index, value)
				}

				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			distance, err := results.Distance()
			So(err, ShouldBeNil)

			out := make([]float64, distance.Len())

			for index := range out {
				out[index] = distance.At(index)
			}

			return out
		}

		Convey("Sympathy asks for less than a cell, none for one cell, repulsion for more", func() {
			// Measured against their root mean square, sqrt(2/3), the
			// strengths 1, 0, -1 read as ±sqrt(3/2).
			relative := math.Sqrt(1.5)
			distance := invert(1, 0, -1)
			So(distance[0], ShouldAlmostEqual, 1/(1+relative))
			So(distance[1], ShouldEqual, 1)
			So(distance[2], ShouldAlmostEqual, 1+relative)

			Convey("And only how strengths compare matters, not their unit", func() {
				scaled := invert(0.01, 0, -0.01)
				for index := range distance {
					So(scaled[index], ShouldAlmostEqual, distance[index])
				}
			})
		})

		Convey("An evaluation with no strength anywhere asks one cell of every pair", func() {
			So(invert(0, 0), ShouldResemble, []float64{1, 1})
		})

		Convey("The next evaluation starts clean", func() {
			invert(1, -1)
			So(invert(), ShouldBeEmpty)
		})
	})
}
