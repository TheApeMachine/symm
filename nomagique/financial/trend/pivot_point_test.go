package trend_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/trend"
)

func TestPivotPoint(t *testing.T) {
	Convey("Given a PivotPoint server and client", t, func() {
		ctx := context.Background()
		server := trend.NewPivotPoint(ctx)
		So(server, ShouldNotBeNil)

		client := trend.PivotPoint_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params trend.PivotPoint_write_Params) error {
				params.SetOpen(100.0)
				params.SetHigh(110.0)
				params.SetLow(90.0)
				params.SetClose(105.0)
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
