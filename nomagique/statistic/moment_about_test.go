package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestMomentAbout(t *testing.T) {
	Convey("Given a MomentAbout server and client", t, func() {
		ctx := context.Background()
		server := statistic.NewMomentAbout(ctx)
		So(server, ShouldNotBeNil)

		client := statistic.MomentAbout_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params statistic.MomentAbout_write_Params) error {
				params.SetValue(4.0)
				params.SetOrder(2.0)
				params.SetMean(2.0)
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
