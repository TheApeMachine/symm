package temporal

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDelayWrite(t *testing.T) {
	Convey("Given a delay read across two series", t, func() {
		ctx := context.Background()
		client := Delay_ServerToClient(NewDelay())
		defer client.Release()

		read := func(scope string, value float64) float64 {
			So(client.Write(ctx, func(params Delay_write_Params) error {
				params.SetValue(value)
				return params.SetScope(scope)
			}), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			return results.Out()
		}

		read("a", 1)
		So(read("a", 2), ShouldEqual, 1)

		Convey("A new series never hands back the previous one's values", func() {
			So(read("b", 7), ShouldEqual, 7)
			So(read("b", 8), ShouldEqual, 7)
		})
	})
}
