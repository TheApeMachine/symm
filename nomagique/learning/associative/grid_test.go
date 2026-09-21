package associative

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAssociativeGrid(t *testing.T) {
	Convey("Given associative Grid primitive", t, func() {
		server := NewGrid()
		client := Grid(Grid_ServerToClient(server))
		defer client.Release()

		ctx := context.Background()

		Convey("When providing initial impulse, it evaluates and resets on Done", func() {
			err := client.Write(ctx, func(params Grid_write_Params) error {
				return params.SetData([]byte("impulse"))
			})
			So(err, ShouldBeNil)

			doneFuture, releaseDone := client.Done(ctx, nil)
			defer releaseDone()

			results, err := doneFuture.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "impulse")
		})
	})
}
