package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestWassersteinDistance(t *testing.T) {
	Convey("Given a WassersteinDistance server and client", t, func() {
		ctx := context.Background()
		server := statistic.NewWassersteinDistance(ctx)
		So(server, ShouldNotBeNil)

		client := statistic.WassersteinDistance_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params statistic.WassersteinDistance_write_Params) error {
				listP, err := params.NewP(3)

				if err != nil {
					return err
				}
				listP.Set(0, 1.0)
				listP.Set(1, 2.0)
				listP.Set(2, 3.0)
				listQ, err := params.NewQ(3)

				if err != nil {
					return err
				}
				listQ.Set(0, 1.5)
				listQ.Set(1, 2.5)
				listQ.Set(2, 3.5)
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
