package temporal

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestElapsedWrite(t *testing.T) {
	Convey("Given elapsed time read across two series", t, func() {
		ctx := context.Background()
		client := Elapsed_ServerToClient(NewElapsed())
		defer client.Release()

		read := func(scope string, at int64) float64 {
			So(client.Write(ctx, func(params Elapsed_write_Params) error {
				params.SetTimestamp(at)
				return params.SetScope(scope)
			}), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			return results.Out()
		}

		So(read("a", 1e9), ShouldEqual, 0)
		So(read("a", 3e9), ShouldEqual, 2)

		Convey("A new series measures nothing against the previous one's clock", func() {
			So(read("b", 100e9), ShouldEqual, 0)
			So(read("b", 101e9), ShouldEqual, 1)
		})
	})
}
