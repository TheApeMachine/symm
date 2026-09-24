package store_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestTrail(t *testing.T) {
	ctx := context.Background()

	Convey("Given a trail that keeps three arrivals", t, func() {
		client := store.Trail_ServerToClient(store.NewTrail())
		defer client.Release()

		step := func(value float64, present bool) string {
			So(client.Write(ctx, func(params store.Trail_write_Params) error {
				params.SetCapacity(3)
				numbers, err := params.NewNumbers(1)

				if err != nil {
					return err
				}

				numbers.Set(0, value)
				flags, err := params.NewPresent(1)

				if err != nil {
					return err
				}

				flags.Set(0, present)
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			if results.Which() != store.Trailed_Which_out {
				return ""
			}

			out, err := results.Out()
			So(err, ShouldBeNil)
			return string(out)
		}

		Convey("It keeps the most recent, oldest first, and ignores what did not arrive", func() {
			So(step(1, true), ShouldEqual, "[1]")
			So(step(9, false), ShouldEqual, "")
			step(2, true)
			step(3, true)
			So(step(4.5, true), ShouldEqual, "[2,3,4.5]")
		})
	})
}
