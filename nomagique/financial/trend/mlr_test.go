package trend_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/trend"
)

func TestMlr(t *testing.T) {
	Convey("Given a Mlr server and client", t, func() {
		ctx := context.Background()
		server := trend.NewMlr(ctx)
		So(server, ShouldNotBeNil)

		client := trend.Mlr_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params trend.Mlr_write_Params) error {
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
