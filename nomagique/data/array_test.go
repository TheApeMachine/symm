package data_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestArray(t *testing.T) {
	ctx := context.Background()

	Convey("Given interleaved numbers", t, func() {
		client := data.Array_ServerToClient(data.NewArray())
		defer client.Release()

		array := func(offset uint32) string {
			So(client.Write(ctx, func(params data.Array_write_Params) error {
				params.SetStride(2)
				params.SetOffset(offset)
				numbers, err := params.NewNumbers(4)

				if err != nil {
					return err
				}

				for index, value := range []float64{1, 10, 2, 20} {
					numbers.Set(index, value)
				}

				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			out, err := results.Out()
			So(err, ShouldBeNil)
			return string(out)
		}

		Convey("Stride and offset take the columns apart", func() {
			So(array(0), ShouldEqual, "[1,2]")
			So(array(1), ShouldEqual, "[10,20]")
		})

		Convey("No list arriving is no array, not an empty one", func() {
			So(client.Write(ctx, func(params data.Array_write_Params) error { return nil }), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			defer release()
			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.HasOut(), ShouldBeFalse)
		})

		Convey("Two lists at once are rejected", func() {
			So(client.Write(ctx, func(params data.Array_write_Params) error {
				if _, err := params.NewNumbers(1); err != nil {
					return err
				}

				texts, err := params.NewTexts(1)

				if err != nil {
					return err
				}

				return texts.Set(0, "a")
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}
