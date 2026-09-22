package trend_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/trend"
)

func TestBoP(t *testing.T) {
	Convey("Given a BoP server and client", t, func() {
		ctx := context.Background()
		server := trend.NewBoP(ctx)
		So(server, ShouldNotBeNil)

		client := trend.BoP_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing open, high, low, close values", func() {
			err := client.Write(ctx, func(params trend.BoP_write_Params) error {
				params.SetOpening(10.0)
				params.SetHigh(15.0)
				params.SetLow(5.0)
				params.SetClose(12.0)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			// BOP = (12 - 10) / (15 - 5) = 2 / 10 = 0.2
			So(results.Result(), ShouldEqual, 0.2)
		})
	})
}
