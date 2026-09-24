package data

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReduce(t *testing.T) {
	ctx := context.Background()

	Convey("Given a fold", t, func() {
		client := Reduce_ServerToClient(NewReduce(ctx))
		defer client.Release()

		type reading struct {
			which Folded_Which
			out   float64
		}

		fold := func(value float64, running, flush bool, scope string) reading {
			So(client.Write(ctx, func(params Reduce_write_Params) error {
				params.SetValue(value)
				params.SetRunning(running)
				params.SetFlush(flush)

				if err := params.SetScope(scope); err != nil {
					return err
				}

				return params.SetOperator("sum")
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			folded := Folded(results)

			if folded.Which() == Folded_Which_idle {
				return reading{which: Folded_Which_idle}
			}

			return reading{which: folded.Which(), out: folded.Out()}
		}

		Convey("A fold that has not been told its collection ended is idle, never zero", func() {
			So(fold(3, false, false, "").which, ShouldEqual, Folded_Which_idle)

			Convey("And the flush publishes what it held with the current value", func() {
				flushed := fold(4, false, true, "")
				So(flushed.which, ShouldEqual, Folded_Which_out)
				So(flushed.out, ShouldEqual, 7)
				So(fold(1, false, false, "").which, ShouldEqual, Folded_Which_idle)
			})
		})

		Convey("A running fold accumulates within its series", func() {
			So(fold(2, true, false, "first").out, ShouldEqual, 2)
			So(fold(5, true, false, "first").out, ShouldEqual, 7)

			Convey("And a new series starts the fold again", func() {
				So(fold(4, true, false, "second").out, ShouldEqual, 4)
				So(fold(1, true, false, "second").out, ShouldEqual, 5)
			})
		})
	})
}
