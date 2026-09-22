package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestBivariateMoment(t *testing.T) {
	Convey("Given a BivariateMoment server and client", t, func() {
		ctx := context.Background()
		server := statistic.NewBivariateMoment(ctx)
		So(server, ShouldNotBeNil)

		client := statistic.BivariateMoment_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params statistic.BivariateMoment_write_Params) error {
				listX, err := params.NewX(3)

				if err != nil {
					return err
				}
				listX.Set(0, 1.0)
				listX.Set(1, 2.0)
				listX.Set(2, 3.0)
				listY, err := params.NewY(3)

				if err != nil {
					return err
				}
				listY.Set(0, 2.0)
				listY.Set(1, 4.0)
				listY.Set(2, 6.0)
				params.SetOrderR(1.0)
				params.SetOrderS(1.0)
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
