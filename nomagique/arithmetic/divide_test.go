package arithmetic

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDivideWrite(t *testing.T) {
	Convey("Given a division", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := Divide_ServerToClient(NewDivide())
		defer client.Release()

		divide := func(a, b float64) (float64, bool) {
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

			if results.Which() != Quotient_Which_out {
				return 0, false
			}

			return results.Out(), true
		}

		Convey("When the divisor is a number", func() {
			Convey("Then the quotient is reported", func() {
				quotient, defined := divide(7, 2)
				So(defined, ShouldBeTrue)
				So(quotient, ShouldAlmostEqual, 3.5, 1e-12)
			})
		})

		Convey("When the divisor is zero", func() {
			Convey("Then the quotient is undefined rather than refused", func() {
				// Refusing would abort the evaluation, so one undefined
				// quotient would erase every other metric measured from the
				// same observation. It is reported undefined, with no
				// infinity passed on.
				for _, numerator := range []float64{1, -1, 0} {
					_, defined := divide(numerator, 0)
					So(defined, ShouldBeFalse)
				}
			})

			Convey("Then the next division still reports its own answer", func() {
				divide(1, 0)
				quotient, defined := divide(9, 3)
				So(defined, ShouldBeTrue)
				So(quotient, ShouldAlmostEqual, 3, 1e-12)
			})
		})
	})
}
