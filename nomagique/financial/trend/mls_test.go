package trend_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/trend"
)

func TestMls(t *testing.T) {
	Convey("Given a Mls server and client", t, func() {
		ctx := context.Background()
		server := trend.NewMls(ctx)
		So(server, ShouldNotBeNil)

		client := trend.Mls_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params trend.Mls_write_Params) error {
				params.SetX(10.0)
				params.SetY(20.0)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.IsValid(), ShouldBeTrue)
		})
	})
}
