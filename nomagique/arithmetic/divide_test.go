package arithmetic

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDivideWrite(t *testing.T) {
	Convey("Given a division", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := Divide_ServerToClient(NewDivide())
		defer client.Release()

		divide := func(a, b float64) float64 {
			err := client.Write(ctx, func(params Divide_write_Params) error {
				params.SetA(a)
				params.SetB(b)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			return results.Out()
		}

		Convey("When the divisor is a number", func() {
			Convey("Then the quotient is reported", func() {
				So(divide(7, 2), ShouldAlmostEqual, 3.5, 1e-12)
			})
		})

		Convey("When the divisor is zero", func() {
			Convey("Then the quotient is undefined rather than refused", func() {
				// Refusing would abort the evaluation, so one undefined
				// quotient would erase every other metric measured from the
				// same observation. It stays undefined and travels alone.
				So(math.IsInf(divide(1, 0), 1), ShouldBeTrue)
				So(math.IsInf(divide(-1, 0), -1), ShouldBeTrue)
				So(math.IsNaN(divide(0, 0)), ShouldBeTrue)
			})

			Convey("Then the next division still reports its own answer", func() {
				divide(1, 0)
				So(divide(9, 3), ShouldAlmostEqual, 3, 1e-12)
			})
		})
	})
}
