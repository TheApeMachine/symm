package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestMahalanobis(t *testing.T) {
	Convey("Given a Mahalanobis server and client", t, func() {
		ctx := context.Background()
		server := statistic.NewMahalanobis(ctx)
		So(server, ShouldNotBeNil)

		client := statistic.Mahalanobis_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params statistic.Mahalanobis_write_Params) error {
				listX, err := params.NewX(2)

				if err != nil {
					return err
				}
				listX.Set(0, 1.0)
				listX.Set(1, 2.0)
				listY, err := params.NewY(2)

				if err != nil {
					return err
				}
				listY.Set(0, 2.0)
				listY.Set(1, 3.0)
				listCholData, err := params.NewCholData(4)

				if err != nil {
					return err
				}
				listCholData.Set(0, 2.0)
				listCholData.Set(1, 1.0)
				listCholData.Set(2, 1.0)
				listCholData.Set(3, 2.0)
				params.SetDim(2)
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
