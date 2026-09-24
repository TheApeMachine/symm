package calculus

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSqrt(t *testing.T) {
	Convey("Given a square root", t, func() {
		ctx := context.Background()
		client := Sqrt_ServerToClient(NewSqrt())
		defer client.Release()

		root := func(value float64) Root {
			So(client.Write(ctx, func(params Sqrt_write_Params) error {
				params.SetValue(value)
				return nil
			}), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			t.Cleanup(release)

			results, err := future.Struct()
			So(err, ShouldBeNil)
			return results
		}

		Convey("A non-negative value has its root", func() {
			result := root(9)
			So(result.Which(), ShouldEqual, Root_Which_out)
			So(result.Out(), ShouldEqual, 3)
		})

		Convey("A negative value has none: the result is undefined, not zero and not a failure", func() {
			So(root(-1).Which(), ShouldEqual, Root_Which_undefined)

			Convey("And the next evaluation starts clean", func() {
				So(root(0).Which(), ShouldEqual, Root_Which_out)
			})
		})
	})
}
