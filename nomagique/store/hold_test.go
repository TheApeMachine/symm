package store

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHold(t *testing.T) {
	ctx := context.Background()

	Convey("Given a hold", t, func() {
		client := Hold_ServerToClient(NewHold())
		defer client.Release()

		type reading struct {
			held  bool
			value float64
			fresh bool
		}

		hold := func(scope string, values []float64, present []bool) reading {
			So(client.Write(ctx, func(params Hold_write_Params) error {
				list, err := params.NewValue(int32(len(values)))

				if err != nil {
					return err
				}

				flags, err := params.NewPresent(int32(len(present)))

				if err != nil {
					return err
				}

				for slot := range values {
					list.Set(slot, values[slot])
					flags.Set(slot, present[slot])
				}

				return params.SetScope(scope)
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()
			results, err := future.Struct()
			So(err, ShouldBeNil)
			held := Held(results)

			if held.Which() == Held_Which_idle {
				return reading{}
			}

			return reading{true, held.Held().Value(), held.Held().Fresh()}
		}

		Convey("Nothing is held before the first reading", func() {
			So(hold("", []float64{0}, []bool{false}), ShouldResemble, reading{})
		})

		Convey("A reading is held, and reported on evaluations that bring none", func() {
			So(hold("", []float64{101}, []bool{true}), ShouldResemble, reading{true, 101, true})
			So(hold("", []float64{0}, []bool{false}), ShouldResemble, reading{true, 101, false})
			So(hold("", nil, nil), ShouldResemble, reading{true, 101, false})

			Convey("And a new series holds nothing until its own first reading", func() {
				So(hold("next", []float64{0}, []bool{false}), ShouldResemble, reading{})
				So(hold("next", []float64{7}, []bool{true}), ShouldResemble, reading{true, 7, true})
			})
		})
	})
}
