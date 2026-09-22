package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestMoment(t *testing.T) {
	Convey("Given a Moment server and client", t, func() {
		ctx := context.Background()
		server := statistic.NewMoment(ctx)
		So(server, ShouldNotBeNil)

		client := statistic.Moment_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params statistic.Moment_write_Params) error {
				params.SetValue(2.0)
				params.SetOrder(3.0)
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
