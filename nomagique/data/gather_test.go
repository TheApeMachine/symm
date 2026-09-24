package data_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestGather(t *testing.T) {
	ctx := context.Background()

	Convey("Given numbers gathered from three producers", t, func() {
		client := data.Gather_ServerToClient(data.NewGather())
		defer client.Release()

		gather := func(values []float64, present []bool) ([]float64, []bool) {
			So(client.Write(ctx, func(params data.Gather_write_Params) error {
				numbers, err := params.NewValues(int32(len(values)))

				if err != nil {
					return err
				}

				flags, err := params.NewPresent(int32(len(present)))

				if err != nil {
					return err
				}

				for slot := range values {
					numbers.Set(slot, values[slot])
					flags.Set(slot, present[slot])
				}

				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, delivered := []float64{}, []bool{}

			if results.Which() == data.Gathered_Which_idle {
				return out, delivered
			}

			numbers, err := results.Gathered().Values()
			So(err, ShouldBeNil)
			flags, err := results.Gathered().Present()
			So(err, ShouldBeNil)

			for slot := range numbers.Len() {
				out = append(out, numbers.At(slot))
				delivered = append(delivered, flags.At(slot))
			}

			return out, delivered
		}

		Convey("The list keeps every slot, and says which producers delivered", func() {
			values, present := gather([]float64{1, 0, 3}, []bool{true, false, true})
			So(values, ShouldResemble, []float64{1, 0, 3})
			So(present, ShouldResemble, []bool{true, false, true})

			Convey("And nothing arriving is no list at all", func() {
				values, present := gather(nil, nil)
				So(values, ShouldBeEmpty)
				So(present, ShouldBeEmpty)
			})

			Convey("And slots none of whose producers delivered are idle", func() {
				values, present := gather([]float64{0, 0, 0}, []bool{false, false, false})
				So(values, ShouldBeEmpty)
				So(present, ShouldBeEmpty)
			})
		})

		Convey("Presence that does not match the values is rejected", func() {
			So(client.Write(ctx, func(params data.Gather_write_Params) error {
				_, err := params.NewValues(2)
				return err
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}
