package store_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestStorePrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given store primitives", t, func() {
		Convey("Constant returns configured payload", func() {
			server := store.NewConstantServer([]byte("hello"))
			client := store.Constant_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params store.Constant_write_Params) error {
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "hello")
		})

		Convey("Radix stores and retrieves bytes", func() {
			server := store.NewRadix()
			client := store.Radix_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params store.Radix_write_Params) error {
				params.SetKey("btc")
				params.SetValue([]byte("65000"))
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Found(), ShouldBeTrue)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "65000")
		})
	})
}
