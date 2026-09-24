package temporal

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLogReturnsWrite(t *testing.T) {
	Convey("Given log returns read across two series", t, func() {
		ctx := context.Background()
		client := LogReturns_ServerToClient(NewLogReturns())
		defer client.Release()

		read := func(scope string, value float64) float64 {
			So(client.Write(ctx, func(params LogReturns_write_Params) error {
				params.SetValue(value)
				return params.SetScope(scope)
			}), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			return results.Out()
		}

		So(read("a", 100), ShouldEqual, 0)
		So(read("a", 110), ShouldAlmostEqual, math.Log(1.1), 1e-12)

		Convey("A new series has no previous value to return against", func() {
			So(read("b", 50), ShouldEqual, 0)
			So(read("b", 25), ShouldAlmostEqual, math.Log(0.5), 1e-12)
		})
	})
}
