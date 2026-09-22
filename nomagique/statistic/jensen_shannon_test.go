package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestJensenShannon(t *testing.T) {
	Convey("Given a JensenShannon server and client", t, func() {
		ctx := context.Background()
		server := statistic.NewJensenShannon(ctx)
		So(server, ShouldNotBeNil)

		client := statistic.JensenShannon_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params statistic.JensenShannon_write_Params) error {
				listP, err := params.NewP(2)

				if err != nil {
					return err
				}
				listP.Set(0, 0.5)
				listP.Set(1, 0.5)
				listQ, err := params.NewQ(2)

				if err != nil {
					return err
				}
				listQ.Set(0, 0.4)
				listQ.Set(1, 0.6)
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
