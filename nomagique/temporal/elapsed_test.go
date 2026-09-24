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

		read := func(scope string, at float64, origin ...bool) float64 {
			So(client.Write(ctx, func(params Elapsed_write_Params) error {
				params.SetTimestamp(at)
				params.SetOrigin(len(origin) > 0 && origin[0])
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

		Convey("Measured from its origin, the time is how long the series has run", func() {
			So(read("c", 10e9, true), ShouldEqual, 0)
			So(read("c", 12e9, true), ShouldEqual, 2)
			So(read("c", 15e9, true), ShouldEqual, 5)
		})
	})
}
